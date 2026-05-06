package parquet

import (
	"fmt"
	"io"
	"os"
	"reflect"
	"time"

	"github.com/parquet-go/parquet-go"
	"github.com/trithemius/parq-vision/internal/config"
)

type DynamicParquetDB struct {
	path      string
	schema    *parquet.Schema
	fieldDefs []config.FieldDef
	rows      []map[string]any
	index     map[string]int
}

func NewDynamicParquetDB(path string, fieldDefs []config.FieldDef) (*DynamicParquetDB, error) {
	schema := buildSchema(fieldDefs)
	db := &DynamicParquetDB{
		path:      path,
		schema:    schema,
		fieldDefs: fieldDefs,
		index:     make(map[string]int),
	}

	if _, err := os.Stat(path); err == nil {
		if err := db.load(); err != nil {
			return nil, fmt.Errorf("failed to load existing parquet file: %w", err)
		}
	}

	return db, nil
}

func buildSchema(fieldDefs []config.FieldDef) *parquet.Schema {
	// Use reflect.StructOf to create a deterministic, ordered schema.
	// Struct fields are ordered, so the resulting Parquet columns will be too.
	sfs := make([]reflect.StructField, 0, 1+len(fieldDefs))
	
	// Column 0: image_path (Required)
	sfs = append(sfs, reflect.StructField{
		Name: "ImagePath",
		Type: reflect.TypeOf(""),
		Tag:  `parquet:"image_path,snappy"`,
	})

	for i, fd := range fieldDefs {
		// Unique exported name for reflect
		name := "F" + fmt.Sprint(i)
		sf := reflect.StructField{
			Name: name,
		}
		
		tag := fd.FieldName
		switch fd.Type {
		case "caption", "free_text":
			sf.Type = reflect.TypeOf("")
			tag += ",snappy"
		case "timestamp", "modified_at":
			t := time.Now()
			sf.Type = reflect.TypeOf(&t) // Pointer makes it Optional
		case "number":
			f := 0.0
			sf.Type = reflect.TypeOf(&f) // Pointer makes it Optional
		}
		sf.Tag = reflect.StructTag(fmt.Sprintf(`parquet:"%s"`, tag))
		sfs = append(sfs, sf)
	}

	typ := reflect.StructOf(sfs)
	// Create a dummy instance to derive schema from
	return parquet.SchemaOf(reflect.New(typ).Elem().Interface())
}

func (db *DynamicParquetDB) load() error {
	file, err := os.Open(db.path)
	if err != nil {
		return err
	}
	defer file.Close()

	// Map field names to column indices in schema
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
			decoded["image_path"] = row[idx].String()
		}
		for _, fd := range db.fieldDefs {
			if idx, ok := colMap[fd.FieldName]; ok {
				decoded[fd.FieldName] = valueToField(row[idx], fd.Type)
			}
		}
		db.index[decoded["image_path"].(string)] = len(db.rows)
		db.rows = append(db.rows, decoded)
		
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
		return v.String()
	case "timestamp", "modified_at":
		return time.Unix(0, v.Int64()).UTC()
	case "number":
		return v.Double()
	default:
		return nil
	}
}

func (db *DynamicParquetDB) AppendRows(newRows []map[string]any, override bool) error {
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
					case "timestamp":
						// Preserve original
					case "free_text", "number":
						// Preserved from original
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
	tempPath := db.path + ".tmp"
	file, err := os.Create(tempPath)
	if err != nil {
		return err
	}

	w := parquet.NewWriter(file, db.schema)
	
	// Map field names to column indices in schema
	colMap := make(map[string]int)
	for i, col := range db.schema.Columns() {
		colMap[col[0]] = i
	}

	rowsToWrite := make([]parquet.Row, 0, len(db.rows))
	for _, r := range db.rows {
		row := make(parquet.Row, len(colMap))
		
		// Column: image_path
		if idx, ok := colMap["image_path"]; ok {
			row[idx] = parquet.ValueOf(r["image_path"]).Level(0, 0, idx)
		}
		
		for _, fd := range db.fieldDefs {
			if idx, ok := colMap[fd.FieldName]; ok {
				val := r[fd.FieldName]
				row[idx] = fieldToValue(val, fd.Type, idx)
			}
		}
		rowsToWrite = append(rowsToWrite, row)
	}

	if _, err := w.WriteRows(rowsToWrite); err != nil {
		w.Close()
		file.Close()
		return err
	}

	if err := w.Close(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}

	return os.Rename(tempPath, db.path)
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
		case "free_text":
			pv = parquet.ValueOf(val.(string))
			dl = 1
		case "timestamp":
			pv = parquet.ValueOf(val.(time.Time).UnixNano())
			dl = 1
		case "modified_at":
			pv = parquet.ValueOf(val.(time.Time).UnixNano())
			dl = 1
		case "number":
			pv = parquet.ValueOf(val.(float64))
			dl = 1
		}
	}
	return pv.Level(0, dl, columnIndex)
}
