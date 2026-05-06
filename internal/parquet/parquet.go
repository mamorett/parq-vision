package parquet

import (
	"fmt"
	"io"
	"os"
	"reflect"
	"time"

	"github.com/parquet-go/parquet-go"
	"github.com/mamorett/parq-vision/internal/config"
)

type DynamicParquetDB struct {
	path      string
	schema    *parquet.Schema
	fieldDefs []config.FieldDef
	rows      []map[string]any
	index     map[string]int
	
	// isReady is only true if we successfully loaded ALL existing data 
	// or if we are starting a fresh file.
	isReady   bool
}

// NewDynamicParquetDB opens an existing parquet file or prepares for a new one.
// It ALWAYS loads existing data if the file exists to prevent data loss.
func NewDynamicParquetDB(path string, fieldDefs []config.FieldDef) (*DynamicParquetDB, error) {
	schema := buildSchema(fieldDefs)
	db := &DynamicParquetDB{
		path:      path,
		schema:    schema,
		fieldDefs: fieldDefs,
		index:     make(map[string]int),
	}

	if _, err := os.Stat(path); err == nil {
		// File exists, we MUST load it successfully before we do anything else.
		if err := db.load(); err != nil {
			return nil, fmt.Errorf("FATAL: Failed to load existing database: %w. Aborting to protect existing data.", err)
		}
	}
	
	db.isReady = true
	return db, nil
}

func buildSchema(fieldDefs []config.FieldDef) *parquet.Schema {
	sfs := make([]reflect.StructField, 0, 1+len(fieldDefs))
	
	// Column 0: image_path (Required, Snappy, DeltaLengthByteArray)
	sfs = append(sfs, reflect.StructField{
		Name: "ImagePath",
		Type: reflect.TypeOf(""),
		Tag:  `parquet:"image_path,snappy,deltalengthbytearray"`,
	})

	for i, fd := range fieldDefs {
		sf := reflect.StructField{
			Name: fmt.Sprintf("F%d", i),
		}
		
		tag := fd.FieldName
		switch fd.Type {
		case "caption":
			sf.Type = reflect.TypeOf("")
			tag += ",snappy,deltalengthbytearray"
		case "free_text":
			sf.Type = reflect.TypeOf("")
			tag += ",snappy,deltalengthbytearray,optional"
		case "timestamp", "modified_at":
			t := time.Now()
			sf.Type = reflect.TypeOf(&t)
		case "number":
			f := 0.0
			sf.Type = reflect.TypeOf(&f)
		}
		sf.Tag = reflect.StructTag(fmt.Sprintf(`parquet:"%s"`, tag))
		sfs = append(sfs, sf)
	}

	typ := reflect.StructOf(sfs)
	return parquet.SchemaOf(reflect.New(typ).Elem().Interface())
}

func (db *DynamicParquetDB) load() error {
	file, err := os.Open(db.path)
	if err != nil {
		return err
	}
	defer file.Close()

	colMap := make(map[string]int)
	for i, col := range db.schema.Columns() {
		colMap[col[0]] = i
	}

	r := parquet.NewReader(file, db.schema)
	for {
		rows := make([]parquet.Row, 1)
		n, err := r.ReadRows(rows)
		if n == 0 {
			if err == io.EOF {
				break
			}
			return err
		}

		row := rows[0]
		decoded := make(map[string]any)
		
		if idx, ok := colMap["image_path"]; ok {
			val := row[idx]
			if !val.IsNull() {
				decoded["image_path"] = string(val.ByteArray())
			}
		}

		for _, fd := range db.fieldDefs {
			if idx, ok := colMap[fd.FieldName]; ok {
				decoded[fd.FieldName] = valueToField(row[idx], fd.Type)
			}
		}

		path, ok := decoded["image_path"].(string)
		if ok && path != "" {
			db.index[path] = len(db.rows)
			db.rows = append(db.rows, decoded)
		}
		
		if err == io.EOF {
			break
		}
	}
	return nil
}

func valueToField(v parquet.Value, fieldType string) any {
	if v.IsNull() {
		return nil
	}
	switch fieldType {
	case "caption", "free_text":
		return string(v.ByteArray())
	case "timestamp", "modified_at":
		return time.Unix(0, v.Int64()).UTC()
	case "number":
		return v.Double()
	default:
		return nil
	}
}

func (db *DynamicParquetDB) AppendRows(newRows []map[string]any, override bool) error {
	if !db.isReady {
		return fmt.Errorf("database protection active: initial load failed")
	}

	for _, row := range newRows {
		imagePath, ok := row["image_path"].(string)
		if !ok {
			return fmt.Errorf("row missing image_path")
		}

		if idx, exists := db.index[imagePath]; exists {
			if override {
				existing := db.rows[idx]
				for _, fd := range db.fieldDefs {
					switch fd.Type {
					case "caption":
						if v, ok := row[fd.FieldName]; ok && v != nil {
							existing[fd.FieldName] = v
						}
					case "modified_at":
						existing[fd.FieldName] = time.Now().UTC()
					}
				}
				db.rows[idx] = existing
			}
		} else {
			db.index[imagePath] = len(db.rows)
			db.rows = append(db.rows, row)
		}
	}
	return nil
}

func (db *DynamicParquetDB) Exists(imagePath string) bool {
	_, ok := db.index[imagePath]
	return ok
}

func (db *DynamicParquetDB) GetRow(imagePath string) (map[string]any, bool) {
	idx, ok := db.index[imagePath]
	if !ok {
		return nil, false
	}
	return db.rows[idx], true
}

func (db *DynamicParquetDB) Close() error {
	return db.Save()
}

func (db *DynamicParquetDB) Save() error {
	if !db.isReady {
		return fmt.Errorf("refusing to save: data integrity not guaranteed")
	}

	tempPath := db.path + ".tmp"
	file, err := os.Create(tempPath)
	if err != nil {
		return err
	}

	w := parquet.NewWriter(file, db.schema)
	
	colMap := make(map[string]int)
	for i, col := range db.schema.Columns() {
		colMap[col[0]] = i
	}

	for _, r := range db.rows {
		row := make(parquet.Row, len(colMap))
		if idx, ok := colMap["image_path"]; ok {
			row[idx] = parquet.ValueOf(r["image_path"]).Level(0, 0, idx)
		}
		for _, fd := range db.fieldDefs {
			if idx, ok := colMap[fd.FieldName]; ok {
				val := r[fd.FieldName]
				row[idx] = fieldToValue(val, fd.Type, idx)
			}
		}
		if _, err := w.WriteRows([]parquet.Row{row}); err != nil {
			w.Close()
			file.Close()
			os.Remove(tempPath)
			return err
		}
	}

	if err := w.Flush(); err != nil {
		w.Close()
		file.Close()
		os.Remove(tempPath)
		return err
	}

	if err := w.Close(); err != nil {
		file.Close()
		os.Remove(tempPath)
		return err
	}
	if err := file.Close(); err != nil {
		os.Remove(tempPath)
		return err
	}

	// Final swap: Original -> Backup, Temp -> Original, Delete Backup
	backupPath := db.path + ".bak"
	exists := false
	if _, err := os.Stat(db.path); err == nil {
		exists = true
		if err := os.Rename(db.path, backupPath); err != nil {
			return fmt.Errorf("failed to create backup: %w", err)
		}
	}

	if err := os.Rename(tempPath, db.path); err != nil {
		if exists {
			os.Rename(backupPath, db.path) // Try to restore
		}
		return fmt.Errorf("failed to finalize save: %w", err)
	}

	if exists {
		os.Remove(backupPath)
	}

	return nil
}

func fieldToValue(val any, fieldType string, columnIndex int) parquet.Value {
	var pv parquet.Value
	dl := 0

	if val == nil {
		pv = parquet.ValueOf(nil)
	} else {
		switch fieldType {
		case "caption":
			pv = parquet.ValueOf(val.(string))
			dl = 0
		case "free_text":
			pv = parquet.ValueOf(val.(string))
			dl = 1
		case "timestamp", "modified_at":
			pv = parquet.ValueOf(val.(time.Time).UnixNano())
			dl = 1
		case "number":
			pv = parquet.ValueOf(val.(float64))
			dl = 1
		}
	}
	return pv.Level(0, dl, columnIndex)
}
