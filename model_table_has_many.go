package bun

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"reflect"

	"github.com/uptrace/bun/internal"
	"github.com/uptrace/bun/schema"
)

type hasManyModel struct {
	*sliceTableModel
	baseTable *schema.Table
	rel       *schema.Relation

	baseValues map[internal.MapKey][]reflect.Value
	structKey  []interface{}
}

var _ TableModel = (*hasManyModel)(nil)

func newHasManyModel(j *relationJoin) *hasManyModel {
	baseTable := j.BaseModel.Table()
	joinModel := j.JoinModel.(*sliceTableModel)
	baseValues := baseValues(joinModel, j.Relation.BasePKs)
	if len(baseValues) == 0 {
		return nil
	}
	m := hasManyModel{
		sliceTableModel: joinModel,
		baseTable:       baseTable,
		rel:             j.Relation,

		baseValues: baseValues,
	}
	if !m.sliceOfPtr {
		m.strct = reflect.New(m.table.Type).Elem()
	}
	return &m
}

func (m *hasManyModel) ScanRows(ctx context.Context, rows *sql.Rows) (int, error) {
	columns, err := rows.Columns()
	if err != nil {
		return 0, err
	}

	m.columns = columns
	dest := makeDest(m, len(columns))

	var n int
	m.structKey = make([]interface{}, len(m.rel.JoinPKs))
	for rows.Next() {
		if m.sliceOfPtr {
			m.strct = reflect.New(m.table.Type).Elem()
		} else {
			m.strct.Set(m.table.ZeroValue)
		}
		m.structInited = false
		m.scanIndex = 0

		if err := rows.Scan(dest...); err != nil {
			return 0, err
		}

		if err := m.parkStruct(); err != nil {
			return 0, err
		}

		n++
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	return n, nil
}

func (m *hasManyModel) Scan(src interface{}) error {
	column := m.columns[m.scanIndex]
	m.scanIndex++

	field := m.table.LookupField(column)
	if field == nil {
		return fmt.Errorf("bun: %s does not have column %q", m.table.TypeName, column)
	}

	if err := field.ScanValue(m.strct, src); err != nil {
		return err
	}

	for i, f := range m.rel.JoinPKs {
		if f.Name == column {
			m.structKey[i] = indirectAsKey(field.Value(m.strct))
			break
		}
	}

	return nil
}

func (m *hasManyModel) parkStruct() error {

	baseValues, ok := m.baseValues[internal.NewMapKey(m.structKey)]
	if !ok {
		return fmt.Errorf(
			"bun: has-many relation=%s does not have base %s with id=%q (check join conditions)",
			m.rel.Field.GoName, m.baseTable, m.structKey)
	}

	for i, v := range baseValues {
		if !m.sliceOfPtr {
			v.Set(reflect.Append(v, m.strct))
			continue
		}

		if i == 0 {
			v.Set(reflect.Append(v, m.strct.Addr()))
			continue
		}

		clone := reflect.New(m.strct.Type()).Elem()
		clone.Set(m.strct)
		v.Set(reflect.Append(v, clone.Addr()))
	}

	return nil
}

func baseValues(model TableModel, fields []*schema.Field) map[internal.MapKey][]reflect.Value {
	fieldIndex := model.Relation().Field.Index
	m := make(map[internal.MapKey][]reflect.Value)

	// Special case for array relationships
	var isArrayRelation bool
	for _, field := range fields {
		if field.Tag.HasOption("array") {
			isArrayRelation = true
			break
		}
	}

	if isArrayRelation {
		// For array relationships, we use parent ID as the key
		// This avoids trying to use an array as a map key
		parentKey := make([]interface{}, 0, 1)
		walk(model.rootValue(), model.parentIndex(), func(v reflect.Value) {
			// Use the parent model's primary key as the map key
			parentKey = parentKey[:0]
			parentID := model.rootValue().FieldByName("ID")
			parentKey = append(parentKey, parentID.Interface())

			mapKey := internal.NewMapKey(parentKey)
			m[mapKey] = append(m[mapKey], v.FieldByIndex(fieldIndex))
		})
		return m
	}

	// Original implementation for non-array relationships
	key := make([]interface{}, 0, len(fields))
	walk(model.rootValue(), model.parentIndex(), func(v reflect.Value) {
		key = modelKey(key[:0], v, fields)
		mapKey := internal.NewMapKey(key)
		m[mapKey] = append(m[mapKey], v.FieldByIndex(fieldIndex))
	})
	return m
}

func modelKey(key []interface{}, strct reflect.Value, fields []*schema.Field) []interface{} {
	for _, f := range fields {
		key = append(key, indirectAsKey(f.Value(strct)))
	}
	return key
}

// indirectAsKey return the field value dereferencing the pointer if necessary.
// The value is then used as a map key.
func indirectAsKey(field reflect.Value) interface{} {
	if field.Kind() == reflect.Pointer && field.IsNil() {
		return nil
	}

	i := field.Interface()
	if valuer, ok := i.(driver.Valuer); ok {
		if v, err := valuer.Value(); err == nil {
			switch reflect.TypeOf(v).Kind() {
			case reflect.Array, reflect.Chan, reflect.Func,
				reflect.Map, reflect.Pointer, reflect.Slice, reflect.UnsafePointer:
				// For complex types, just use the interface value directly
				return i
			default:
				return v
			}
		}
	}

	// For simplicity, just return the indirect value instead of special handling for arrays
	return reflect.Indirect(field).Interface()
}
