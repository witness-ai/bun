package bun

import (
	"context"
	"reflect"
	"time"

	"github.com/uptrace/bun/dialect"
	"github.com/uptrace/bun/dialect/feature"
	"github.com/uptrace/bun/internal"
	"github.com/uptrace/bun/schema"
)

type relationJoin struct {
	Parent    *relationJoin
	BaseModel TableModel
	JoinModel TableModel
	Relation  *schema.Relation

	apply   func(*SelectQuery) *SelectQuery
	columns []schema.QueryWithArgs
}

func (j *relationJoin) applyTo(q *SelectQuery) {
	if j.apply == nil {
		return
	}

	var table *schema.Table
	var columns []schema.QueryWithArgs

	// Save state.
	table, q.table = q.table, j.JoinModel.Table()
	columns, q.columns = q.columns, nil

	q = j.apply(q)

	// Restore state.
	q.table = table
	j.columns, q.columns = q.columns, columns
}

func (j *relationJoin) Select(ctx context.Context, q *SelectQuery) error {
	switch j.Relation.Type {
	}
	panic("not reached")
}

func (j *relationJoin) selectMany(ctx context.Context, q *SelectQuery) error {
	q = j.manyQuery(q)
	if q == nil {
		return nil
	}
	return q.Scan(ctx)
}

func (j *relationJoin) manyQuery(q *SelectQuery) *SelectQuery {
	hasManyModel := newHasManyModel(j)
	if hasManyModel == nil {
		return nil
	}

	q = q.Model(hasManyModel)

	// Handle array relations differently - use PostgreSQL's ANY operator
	if j.Relation.IsArray {
		return j.buildArrayQuery(q, j.Relation.BaseFields)
	}

	// Standard approach for non-array relations
	var where []byte

	if q.db.dialect.Features().Has(feature.CompositeIn) {
		return j.manyQueryCompositeIn(where, q)
	}
	return j.manyQueryMulti(where, q)
}

func (j *relationJoin) manyQueryCompositeIn(where []byte, q *SelectQuery) *SelectQuery {
	if len(j.Relation.JoinFields) > 1 {
		where = append(where, '(')
	}
	where = appendColumns(where, j.JoinModel.Table().SQLAlias, j.Relation.JoinFields)
	if len(j.Relation.JoinFields) > 1 {
		where = append(where, ')')
	}
	where = append(where, " IN ("...)
	where = appendChildValues(
		q.db.Formatter(),
		where,
		j.JoinModel.rootValue(),
		j.JoinModel.parentIndex(),
		j.Relation.BaseFields,
	)
	where = append(where, ")"...)
	q = q.Where(internal.String(where))

	if j.Relation.PolymorphicField != nil {
		q = q.Where("? = ?", j.Relation.PolymorphicField.SQLName, j.Relation.PolymorphicValue)
	}

	j.applyTo(q)
	q = q.Apply(j.hasManyColumns)

	return q
}

func (j *relationJoin) manyQueryMulti(where []byte, q *SelectQuery) *SelectQuery {
	where = appendMultiValues(
		q.db.Formatter(),
		where,
		j.JoinModel.rootValue(),
		j.JoinModel.parentIndex(),
		j.Relation.BaseFields,
		j.Relation.JoinFields,
		j.JoinModel.Table().SQLAlias,
	)

	q = q.Where(internal.String(where))

	if j.Relation.PolymorphicField != nil {
		q = q.Where("? = ?", j.Relation.PolymorphicField.SQLName, j.Relation.PolymorphicValue)
	}

	j.applyTo(q)
	q = q.Apply(j.hasManyColumns)

	return q
}

func (j *relationJoin) hasManyColumns(q *SelectQuery) *SelectQuery {
	b := make([]byte, 0, 32)

	joinTable := j.JoinModel.Table()
	if len(j.columns) > 0 {
		for i, col := range j.columns {
			if i > 0 {
				b = append(b, ", "...)
			}

			if col.Args == nil {
				if field, ok := joinTable.FieldMap[col.Query]; ok {
					b = append(b, joinTable.SQLAlias...)
					b = append(b, '.')
					b = append(b, field.SQLName...)
					continue
				}
			}

			var err error
			b, err = col.AppendQuery(q.db.fmter, b)
			if err != nil {
				q.setErr(err)
				return q
			}

		}
	} else {
		b = appendColumns(b, joinTable.SQLAlias, joinTable.Fields)
	}

	q = q.ColumnExpr(internal.String(b))

	return q
}

func (j *relationJoin) selectM2M(ctx context.Context, q *SelectQuery) error {
	q = j.m2mQuery(q)
	if q == nil {
		return nil
	}
	return q.Scan(ctx)
}

func (j *relationJoin) m2mQuery(q *SelectQuery) *SelectQuery {
	m2mModel := newM2MModel(j)
	if m2mModel == nil {
		return nil
	}

	q = q.Model(m2mModel)

	// Handle array relations differently - use PostgreSQL's ANY operator
	if j.Relation.IsArray {
		return j.buildArrayQuery(q, j.Relation.M2MBaseFields)
	}

	// Standard approach for non-array relations
	baseTable := j.BaseModel.Table()
	m2mTable := j.Relation.M2MTable

	index := j.JoinModel.parentIndex()

	if m2mTable != nil {
		fields := append(j.Relation.M2MBaseFields, j.Relation.M2MJoinFields...)

		b := make([]byte, 0, len(fields))
		b = appendColumns(b, m2mTable.SQLAlias, fields)

		q = q.ColumnExpr(internal.String(b))
	}

	//nolint
	var join []byte
	join = append(join, "JOIN "...)
	join = q.db.fmter.AppendQuery(join, string(m2mTable.SQLName))
	join = append(join, " AS "...)
	join = append(join, m2mTable.SQLAlias...)
	join = append(join, " ON ("...)
	for i, col := range j.Relation.M2MBaseFields {
		if i > 0 {
			join = append(join, ", "...)
		}
		join = append(join, m2mTable.SQLAlias...)
		join = append(join, '.')
		join = append(join, col.SQLName...)
	}
	join = append(join, ") IN ("...)
	join = appendChildValues(q.db.fmter, join, j.BaseModel.rootValue(), index, baseTable.PKs)
	join = append(join, ")"...)
	q = q.Join(internal.String(join))

	joinTable := j.JoinModel.Table()
	for i, m2mJoinField := range j.Relation.M2MJoinFields {
		joinField := j.Relation.JoinFields[i]
		q = q.Where("?.? = ?.?",
			joinTable.SQLAlias, joinField.SQLName,
			m2mTable.SQLAlias, m2mJoinField.SQLName)
	}

	j.applyTo(q)
	q = q.Apply(j.hasManyColumns)

	return q
}

func (j *relationJoin) hasParent() bool {
	if j.Parent != nil {
		switch j.Parent.Relation.Type {
		case schema.HasOneRelation, schema.BelongsToRelation:
			return true
		}
	}
	return false
}

func (j *relationJoin) appendAlias(fmter schema.Formatter, b []byte) []byte {
	quote := fmter.IdentQuote()

	b = append(b, quote)
	b = appendAlias(b, j)
	b = append(b, quote)
	return b
}

func (j *relationJoin) appendAliasColumn(fmter schema.Formatter, b []byte, column string) []byte {
	quote := fmter.IdentQuote()

	b = append(b, quote)
	b = appendAlias(b, j)
	b = append(b, "__"...)
	b = append(b, column...)
	b = append(b, quote)
	return b
}

func (j *relationJoin) appendBaseAlias(fmter schema.Formatter, b []byte) []byte {
	quote := fmter.IdentQuote()

	if j.hasParent() {
		b = append(b, quote)
		b = appendAlias(b, j.Parent)
		b = append(b, quote)
		return b
	}
	return append(b, j.BaseModel.Table().SQLAlias...)
}

func (j *relationJoin) appendSoftDelete(
	fmter schema.Formatter, b []byte, flags internal.Flag,
) []byte {
	b = append(b, '.')

	field := j.JoinModel.Table().SoftDeleteField
	b = append(b, field.SQLName...)

	if field.IsPtr || field.NullZero {
		if flags.Has(deletedFlag) {
			b = append(b, " IS NOT NULL"...)
		} else {
			b = append(b, " IS NULL"...)
		}
	} else {
		if flags.Has(deletedFlag) {
			b = append(b, " != "...)
		} else {
			b = append(b, " = "...)
		}
		b = fmter.Dialect().AppendTime(b, time.Time{})
	}

	return b
}

func appendAlias(b []byte, j *relationJoin) []byte {
	if j.hasParent() {
		b = appendAlias(b, j.Parent)
		b = append(b, "__"...)
	}
	b = append(b, j.Relation.Field.Name...)
	return b
}

func (j *relationJoin) appendHasOneJoin(
	fmter schema.Formatter, b []byte, q *SelectQuery,
) (_ []byte, err error) {
	isSoftDelete := j.JoinModel.Table().SoftDeleteField != nil && !q.flags.Has(allWithDeletedFlag)

	b = append(b, "LEFT JOIN "...)
	b = fmter.AppendQuery(b, string(j.JoinModel.Table().SQLNameForSelects))
	b = append(b, " AS "...)
	b = j.appendAlias(fmter, b)

	b = append(b, " ON "...)

	b = append(b, '(')
	for i, baseField := range j.Relation.BaseFields {
		if i > 0 {
			b = append(b, " AND "...)
		}
		b = j.appendAlias(fmter, b)
		b = append(b, '.')
		b = append(b, j.Relation.JoinFields[i].SQLName...)
		b = append(b, " = "...)
		b = j.appendBaseAlias(fmter, b)
		b = append(b, '.')
		b = append(b, baseField.SQLName...)
	}
	b = append(b, ')')

	if isSoftDelete {
		b = append(b, " AND "...)
		b = j.appendAlias(fmter, b)
		b = j.appendSoftDelete(fmter, b, q.flags)
	}

	return b, nil
}

func appendChildValues(
	fmter schema.Formatter, b []byte, v reflect.Value, index []int, fields []*schema.Field,
) []byte {
	seen := make(map[string]struct{})
	walk(v, index, func(v reflect.Value) {
		start := len(b)

		if len(fields) > 1 {
			b = append(b, '(')
		}
		for i, f := range fields {
			if i > 0 {
				b = append(b, ", "...)
			}
			b = f.AppendValue(fmter, b, v)
		}
		if len(fields) > 1 {
			b = append(b, ')')
		}
		b = append(b, ", "...)

		if _, ok := seen[string(b[start:])]; ok {
			b = b[:start]
		} else {
			seen[string(b[start:])] = struct{}{}
		}
	})
	if len(seen) > 0 {
		b = b[:len(b)-2] // trim ", "
	}
	return b
}

// appendMultiValues is an alternative to appendChildValues that doesn't use the sql keyword ID
// but instead use a old style ((k1=v1) AND (k2=v2)) OR (...) of conditions.
func appendMultiValues(
	fmter schema.Formatter, b []byte, v reflect.Value, index []int, baseFields, joinFields []*schema.Field, joinTable schema.Safe,
) []byte {
	// This is based on a mix of appendChildValues and query_base.appendColumns

	// These should never missmatch in length but nice to know if it does
	if len(joinFields) != len(baseFields) {
		panic("not reached")
	}

	// walk the relations
	b = append(b, '(')
	seen := make(map[string]struct{})
	walk(v, index, func(v reflect.Value) {
		start := len(b)
		for i, f := range baseFields {
			if i > 0 {
				b = append(b, " AND "...)
			}
			if len(baseFields) > 1 {
				b = append(b, '(')
			}
			// Field name
			b = append(b, joinTable...)
			b = append(b, '.')
			b = append(b, []byte(joinFields[i].SQLName)...)

			// Equals value
			b = append(b, '=')
			b = f.AppendValue(fmter, b, v)
			if len(baseFields) > 1 {
				b = append(b, ')')
			}
		}

		b = append(b, ") OR ("...)

		if _, ok := seen[string(b[start:])]; ok {
			b = b[:start]
		} else {
			seen[string(b[start:])] = struct{}{}
		}
	})
	if len(seen) > 0 {
		b = b[:len(b)-6] // trim ") OR ("
	}
	b = append(b, ')')
	return b
}

func (j *relationJoin) buildArrayQuery(q *SelectQuery, fields []*schema.Field) *SelectQuery {
	// Arrays are primarily supported in PostgreSQL
	if q.db.dialect.Name() != dialect.PG {
		return nil // Only PostgreSQL is fully supported for array relations
	}

	if len(fields) == 0 {
		return nil // No fields to work with
	}

	joinTable := j.JoinModel.Table()
	if len(joinTable.PKs) == 0 {
		return nil // Join table has no PKs
	}

	baseTable := j.BaseModel.Table()

	// For array relations, the fields parameter contains the reference fields parsed from the join tag
	// The first field is the array field in the base table
	arrayField := fields[0]

	// The join field is the primary key of the join table
	joinPK := joinTable.PKs[0]

	// Use PostgreSQL's ANY operator with proper field references
	q = q.Where("?.? = ANY(?.?)",
		joinTable.SQLAlias, joinPK.SQLName,
		baseTable.SQLAlias, arrayField.SQLName)

	// Add LIMIT 1 for has-one relations
	if j.Relation.Type == schema.HasOneRelation {
		q = q.Limit(1)
	}

	j.applyTo(q)

	// Apply appropriate columns based on relation type
	switch j.Relation.Type {
	case schema.HasOneRelation, schema.BelongsToRelation:
		// For has-one and belongs-to, no column application needed
	case schema.HasManyRelation, schema.ManyToManyRelation:
		q = q.Apply(j.hasManyColumns)
	}

	return q
}

// selectSingleRecord handles loading a single relation (has-one or belongs-to) using array columns
func (j *relationJoin) selectSingleRecord(ctx context.Context, q *SelectQuery, queryBuilder func(*SelectQuery) *SelectQuery) error {
	q = queryBuilder(q)
	if q == nil {
		return nil
	}
	return q.Scan(ctx)
}

// newStructModel creates a struct model for a relation (used by has-one and belongs-to)
func newStructModel(j *relationJoin) TableModel {
	// For a relation to a single record, we need to create a struct model
	// similar to how hasManyModel and m2mModel are created

	// Create a new instance of the struct
	strct := reflect.New(j.JoinModel.Table().Type).Elem()

	// Get the DB reference from the join model, safely checking the type
	var db *DB
	if stm, ok := j.JoinModel.(*structTableModel); ok {
		db = stm.db
	} else {
		// If it's not a structTableModel, fall back to using BaseModel
		if stm, ok := j.BaseModel.(*structTableModel); ok {
			db = stm.db
		} else {
			// Without a DB reference, we can't create a proper model
			return nil
		}
	}

	// Create a table model from the struct
	tm := &structTableModel{
		db:    db,
		table: j.JoinModel.Table(),
		rel:   j.Relation,
		strct: strct,
		dest:  strct.Addr().Interface(),
	}

	return tm
}

// singleRecordQuery builds a query for has-one or belongs-to relations using array columns
func (j *relationJoin) singleRecordQuery(q *SelectQuery) *SelectQuery {
	// Create a model for the related record
	structModel := newStructModel(j)
	if structModel == nil {
		return nil
	}

	q = q.Model(structModel)

	// Use the buildArrayQuery function which handles array relations
	// This handles LIMIT 1 for has-one relations based on relation type
	return j.buildArrayQuery(q, j.Relation.BaseFields)
}

// hasOneQuery builds a query for has-one relations
func (j *relationJoin) hasOneQuery(q *SelectQuery) *SelectQuery {
	// If this is an array relation, use the array-specific query builder
	if j.Relation.IsArray {
		return j.singleRecordQuery(q)
	}
	
	// For non-array relations, use standard query approach
	return j.singleRecordQuery(q)
}

// belongsToQuery builds a query for belongs-to relations
func (j *relationJoin) belongsToQuery(q *SelectQuery) *SelectQuery {
	// If this is an array relation, use the array-specific query builder
	if j.Relation.IsArray {
		return j.singleRecordQuery(q)
	}
	
	// For non-array relations, use standard query approach
	return j.singleRecordQuery(q)
}
