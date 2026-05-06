package parquet

import (
	"fmt"
	"io"
	"os"
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
	// Column 0: image_path — exact same node as current code
	nodes := []parquet.Node{
		parquet.Encoded(
			parquet.Compressed(parquet.String(), &parquet.Snappy),
			&parquet.DeltaLengthByteArray,
		),
	}
	for _, fd := range fieldDefs {
		var node parquet.Node
		switch fd.Type {
		case "caption":
			node = parquet.Encoded(
				parquet.Compressed(parquet.String(), &parquet.Snappy),
				&parquet.DeltaLengthByteArray,
			)
		case "free_text":
			node = parquet.Optional(parquet.Encoded(
				parquet.Compressed(parquet.String(), &parquet.Snappy),
				&parquet.DeltaLengthByteArray,
			))
		case "timestamp":
			node = parquet.Optional(parquet.Timestamp(parquet.Nanosecond))
		case "modified_at":
			node = parquet.Optional(parquet.Timestamp(parquet.Nanosecond))
		case "number":
			node = parquet.Optional(parquet.Leaf(parquet.DoubleType))
		}
		nodes = append(nodes, node)
	}

	// Create a group node with ordered children. 
	// Since parquet.Group is a map, we use it to define names, but the schema 
	// order is determined by how we write the rows and how the reader interprets them.
	// Actually, parquet.NewSchema takes a Node. A Group node IS a Node.
	group := make(parquet.Group)
	group["image_path"] = nodes[0]
	for i, fd := range fieldDefs {
		group[fd.FieldName] = nodes[i+1]
	}

	return parquet.NewSchema("parq-vision", group)
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
		if _, err := w.WriteRows([]parquet.Row{row}); err != nil {
			w.Close()
			file.Close()
			return err
		}
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
