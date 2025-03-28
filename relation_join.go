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

	additionalJoinOnConditions []schema.QueryWithArgs

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

	var where []byte

	if q.db.HasFeature(feature.CompositeIn) {
		return j.manyQueryCompositeIn(where, q)
	}
	return j.manyQueryMulti(where, q)
}

func (j *relationJoin) manyQueryCompositeIn(where []byte, q *SelectQuery) *SelectQuery {
	// Check if this is an array relationship where the join field is an array type
	if j.Relation.IsArrayRelation {
		// Handle array relationship differently
		var arrayOnBase bool

		// Determine which side has the array
		for _, field := range j.Relation.BasePKs {
			if field.Tag.HasOption("array") {
				arrayOnBase = true
				break
			}
		}

		if arrayOnBase {
			// Base has array field, use = ANY operator
			for i, baseField := range j.Relation.BasePKs {
				if baseField.Tag.HasOption("array") {
					if i > 0 {
						where = append(where, " AND "...)
					}
					where = append(where, j.JoinModel.Table().SQLAlias...)
					where = append(where, '.')
					where = append(where, j.Relation.JoinPKs[i].SQLName...)
					where = append(where, " = ANY("...)
					where = appendColumnValue(
						q.db.Formatter(),
						where,
						j.JoinModel.rootValue(),
						j.JoinModel.parentIndex(),
						baseField,
					)
					where = append(where, ")"...)
				} else {
					// Standard condition for non-array fields
					if i > 0 {
						where = append(where, " AND "...)
					}
					where = append(where, j.JoinModel.Table().SQLAlias...)
					where = append(where, '.')
					where = append(where, j.Relation.JoinPKs[i].SQLName...)
					where = append(where, " = "...)
					where = appendColumnValue(
						q.db.Formatter(),
						where,
						j.JoinModel.rootValue(),
						j.JoinModel.parentIndex(),
						baseField,
					)
				}
			}
		} else {
			// Join has array field, use = ANY operator
			for i, joinField := range j.Relation.JoinPKs {
				if joinField.Tag.HasOption("array") {
					if i > 0 {
						where = append(where, " AND "...)
					}
					where = appendColumnValue(
						q.db.Formatter(),
						where,
						j.JoinModel.rootValue(),
						j.JoinModel.parentIndex(),
						j.Relation.BasePKs[i],
					)
					where = append(where, " = ANY("...)
					where = append(where, j.JoinModel.Table().SQLAlias...)
					where = append(where, '.')
					where = append(where, joinField.SQLName...)
					where = append(where, ")"...)
				} else {
					// Standard condition for non-array fields
					if i > 0 {
						where = append(where, " AND "...)
					}
					where = append(where, j.JoinModel.Table().SQLAlias...)
					where = append(where, '.')
					where = append(where, joinField.SQLName...)
					where = append(where, " = "...)
					where = appendColumnValue(
						q.db.Formatter(),
						where,
						j.JoinModel.rootValue(),
						j.JoinModel.parentIndex(),
						j.Relation.BasePKs[i],
					)
				}
			}
		}
	} else {
		// Original implementation for non-array relationships
		// Check if any of the base PKs is an array field
		var hasArrayField bool
		for _, field := range j.Relation.BasePKs {
			if field.Tag.HasOption("array") {
				hasArrayField = true
				break
			}
		}

		if hasArrayField && q.db.Dialect().Name() == dialect.PG {
			// For PostgreSQL with array fields, we need a special approach
			if len(j.Relation.BasePKs) == 1 && j.Relation.BasePKs[0].Tag.HasOption("array") {
				// Simple case: single array field
				where = append(where, j.JoinModel.Table().SQLAlias...)
				where = append(where, '.')
				where = append(where, j.Relation.JoinPKs[0].SQLName...)
				where = append(where, " = ANY("...)
				where = appendColumnValue(
					q.db.Formatter(),
					where,
					j.JoinModel.rootValue(),
					j.JoinModel.parentIndex(),
					j.Relation.BasePKs[0],
				)
				where = append(where, ")"...)
			} else {
				// Multiple fields or complex case - use standard approach for now
				// but will need more work for full array support
				if len(j.Relation.JoinPKs) > 1 {
					where = append(where, '(')
				}
				where = appendColumns(where, j.JoinModel.Table().SQLAlias, j.Relation.JoinPKs)
				if len(j.Relation.JoinPKs) > 1 {
					where = append(where, ')')
				}
				where = append(where, " IN ("...)
				where = appendChildValues(
					q.db.Formatter(),
					where,
					j.JoinModel.rootValue(),
					j.JoinModel.parentIndex(),
					j.Relation.BasePKs,
				)
				where = append(where, ")"...)
			}
		} else {
			// Standard non-array implementation
			if len(j.Relation.JoinPKs) > 1 {
				where = append(where, '(')
			}
			where = appendColumns(where, j.JoinModel.Table().SQLAlias, j.Relation.JoinPKs)
			if len(j.Relation.JoinPKs) > 1 {
				where = append(where, ')')
			}
			where = append(where, " IN ("...)
			where = appendChildValues(
				q.db.Formatter(),
				where,
				j.JoinModel.rootValue(),
				j.JoinModel.parentIndex(),
				j.Relation.BasePKs,
			)
			where = append(where, ")"...)
		}
	}

	if len(j.additionalJoinOnConditions) > 0 {
		where = append(where, " AND "...)
		where = appendAdditionalJoinOnConditions(q.db.Formatter(), where, j.additionalJoinOnConditions)
	}

	q = q.Where(internal.String(where))

	if j.Relation.PolymorphicField != nil {
		q = q.Where("? = ?", j.Relation.PolymorphicField.SQLName, j.Relation.PolymorphicValue)
	}

	j.applyTo(q)
	q = q.Apply(j.hasManyColumns)

	return q
}

func (j *relationJoin) manyQueryMulti(where []byte, q *SelectQuery) *SelectQuery {
	if j.Relation.IsArrayRelation {
		// Use the same logic as manyQueryCompositeIn for array relationships
		var arrayOnBase bool

		// Determine which side has the array
		for _, field := range j.Relation.BasePKs {
			if field.Tag.HasOption("array") {
				arrayOnBase = true
				break
			}
		}

		where = append(where, '(')

		if arrayOnBase {
			// Base has array field, use = ANY operator
			for i, baseField := range j.Relation.BasePKs {
				if baseField.Tag.HasOption("array") {
					if i > 0 {
						where = append(where, " AND "...)
					}
					where = append(where, j.JoinModel.Table().SQLAlias...)
					where = append(where, '.')
					where = append(where, j.Relation.JoinPKs[i].SQLName...)
					where = append(where, " = ANY("...)
					where = appendColumnValue(
						q.db.Formatter(),
						where,
						j.JoinModel.rootValue(),
						j.JoinModel.parentIndex(),
						baseField,
					)
					where = append(where, ")"...)
				} else {
					// Standard condition for non-array fields
					if i > 0 {
						where = append(where, " AND "...)
					}
					where = append(where, j.JoinModel.Table().SQLAlias...)
					where = append(where, '.')
					where = append(where, j.Relation.JoinPKs[i].SQLName...)
					where = append(where, " = "...)
					where = appendColumnValue(
						q.db.Formatter(),
						where,
						j.JoinModel.rootValue(),
						j.JoinModel.parentIndex(),
						baseField,
					)
				}
			}
		} else {
			// Join has array field, use = ANY operator
			for i, joinField := range j.Relation.JoinPKs {
				if joinField.Tag.HasOption("array") {
					if i > 0 {
						where = append(where, " AND "...)
					}
					where = appendColumnValue(
						q.db.Formatter(),
						where,
						j.JoinModel.rootValue(),
						j.JoinModel.parentIndex(),
						j.Relation.BasePKs[i],
					)
					where = append(where, " = ANY("...)
					where = append(where, j.JoinModel.Table().SQLAlias...)
					where = append(where, '.')
					where = append(where, joinField.SQLName...)
					where = append(where, ")"...)
				} else {
					// Standard condition for non-array fields
					if i > 0 {
						where = append(where, " AND "...)
					}
					where = append(where, j.JoinModel.Table().SQLAlias...)
					where = append(where, '.')
					where = append(where, joinField.SQLName...)
					where = append(where, " = "...)
					where = appendColumnValue(
						q.db.Formatter(),
						where,
						j.JoinModel.rootValue(),
						j.JoinModel.parentIndex(),
						j.Relation.BasePKs[i],
					)
				}
			}
		}

		where = append(where, ')')
	} else {
		// Check if any of the base PKs is an array field
		var hasArrayField bool
		for _, field := range j.Relation.BasePKs {
			if field.Tag.HasOption("array") {
				hasArrayField = true
				break
			}
		}

		if hasArrayField && q.db.Dialect().Name() == dialect.PG {
			// For PostgreSQL with array fields, we need a special approach
			if len(j.Relation.BasePKs) == 1 && j.Relation.BasePKs[0].Tag.HasOption("array") {
				// Simple case: single array field
				where = append(where, j.JoinModel.Table().SQLAlias...)
				where = append(where, '.')
				where = append(where, j.Relation.JoinPKs[0].SQLName...)
				where = append(where, " = ANY("...)
				where = appendColumnValue(
					q.db.Formatter(),
					where,
					j.JoinModel.rootValue(),
					j.JoinModel.parentIndex(),
					j.Relation.BasePKs[0],
				)
				where = append(where, ")"...)
			} else {
				// Multiple fields or complex case - use standard approach for now
				// but will need more work for full array support
				if len(j.Relation.JoinPKs) > 1 {
					where = append(where, '(')
				}
				where = appendColumns(where, j.JoinModel.Table().SQLAlias, j.Relation.JoinPKs)
				if len(j.Relation.JoinPKs) > 1 {
					where = append(where, ')')
				}
				where = append(where, " IN ("...)
				where = appendChildValues(
					q.db.Formatter(),
					where,
					j.JoinModel.rootValue(),
					j.JoinModel.parentIndex(),
					j.Relation.BasePKs,
				)
				where = append(where, ")"...)
			}
		} else {
			// Original implementation for non-array relationships
			where = append(where, '(')
			where = appendMultiValues(
				q.db.Formatter(),
				where,
				j.JoinModel.rootValue(),
				j.JoinModel.parentIndex(),
				j.Relation.BasePKs,
				j.Relation.JoinPKs,
				j.JoinModel.Table().SQLAlias,
			)
			where = append(where, ')')
		}
	}

	if len(j.additionalJoinOnConditions) > 0 {
		where = append(where, " AND "...)
		where = appendAdditionalJoinOnConditions(q.db.Formatter(), where, j.additionalJoinOnConditions)
	}

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
	fmter := q.db.fmter

	m2mModel := newM2MModel(j)
	if m2mModel == nil {
		return nil
	}
	q = q.Model(m2mModel)

	index := j.JoinModel.parentIndex()

	if j.Relation.M2MTable != nil {
		// We only need base pks to park joined models to the base model.
		fields := j.Relation.M2MBasePKs

		b := make([]byte, 0, len(fields))
		b = appendColumns(b, j.Relation.M2MTable.SQLAlias, fields)

		q = q.ColumnExpr(internal.String(b))
	}

	//nolint
	var join []byte
	join = append(join, "JOIN "...)
	join = fmter.AppendQuery(join, string(j.Relation.M2MTable.SQLName))
	join = append(join, " AS "...)
	join = append(join, j.Relation.M2MTable.SQLAlias...)
	join = append(join, " ON ("...)
	for i, col := range j.Relation.M2MBasePKs {
		if i > 0 {
			join = append(join, ", "...)
		}
		join = append(join, j.Relation.M2MTable.SQLAlias...)
		join = append(join, '.')
		join = append(join, col.SQLName...)
	}
	join = append(join, ") IN ("...)
	join = appendChildValues(fmter, join, j.BaseModel.rootValue(), index, j.Relation.BasePKs)
	join = append(join, ")"...)

	if len(j.additionalJoinOnConditions) > 0 {
		join = append(join, " AND "...)
		join = appendAdditionalJoinOnConditions(fmter, join, j.additionalJoinOnConditions)
	}

	q = q.Join(internal.String(join))

	joinTable := j.JoinModel.Table()
	for i, m2mJoinField := range j.Relation.M2MJoinPKs {
		joinField := j.Relation.JoinPKs[i]

		// Handle array relationships in M2M joins
		if j.Relation.IsArrayRelation {
			// Check which field is an array type
			if joinField.Tag.HasOption("array") {
				// Join field is an array, use the ANY function
				q = q.Where("?.? = ANY(?.?)",
					j.Relation.M2MTable.SQLAlias, m2mJoinField.SQLName,
					joinTable.SQLAlias, joinField.SQLName)
			} else if m2mJoinField.Tag.HasOption("array") {
				// M2M join field is an array, use the array contains operator
				q = q.Where("?.? @> ARRAY[?.?]",
					j.Relation.M2MTable.SQLAlias, m2mJoinField.SQLName,
					joinTable.SQLAlias, joinField.SQLName)
			} else {
				// Standard equality join
				q = q.Where("?.? = ?.?",
					joinTable.SQLAlias, joinField.SQLName,
					j.Relation.M2MTable.SQLAlias, m2mJoinField.SQLName)
			}
		} else {
			// Standard non-array join
			q = q.Where("?.? = ?.?",
				joinTable.SQLAlias, joinField.SQLName,
				j.Relation.M2MTable.SQLAlias, m2mJoinField.SQLName)
		}
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

	// Handle the case where one of the fields is a PostgreSQL array type
	if j.Relation.IsArrayRelation {
		// Check which side of the relation is the array
		for i, baseField := range j.Relation.BasePKs {
			if i > 0 {
				b = append(b, " AND "...)
			}

			// Check if base field is an array
			if baseField.Tag.HasOption("array") {
				// For array fields, use the PostgreSQL array contains (@>) operator
				// or the 'ANY' function depending on which side is the array
				b = j.appendBaseAlias(fmter, b)
				b = append(b, '.')
				b = append(b, baseField.SQLName...)
				b = append(b, " @> ARRAY["...)
				b = j.appendAlias(fmter, b)
				b = append(b, '.')
				b = append(b, j.Relation.JoinPKs[i].SQLName...)
				b = append(b, "]"...)
			} else if j.Relation.JoinPKs[i].Tag.HasOption("array") {
				// Join field is an array, use the ANY function
				b = j.appendBaseAlias(fmter, b)
				b = append(b, '.')
				b = append(b, baseField.SQLName...)
				b = append(b, " = ANY("...)
				b = j.appendAlias(fmter, b)
				b = append(b, '.')
				b = append(b, j.Relation.JoinPKs[i].SQLName...)
				b = append(b, ")"...)
			} else {
				// Standard equality join
				b = j.appendAlias(fmter, b)
				b = append(b, '.')
				b = append(b, j.Relation.JoinPKs[i].SQLName...)
				b = append(b, " = "...)
				b = j.appendBaseAlias(fmter, b)
				b = append(b, '.')
				b = append(b, baseField.SQLName...)
			}
		}
	} else {
		// Standard non-array join
		for i, baseField := range j.Relation.BasePKs {
			if i > 0 {
				b = append(b, " AND "...)
			}
			b = j.appendAlias(fmter, b)
			b = append(b, '.')
			b = append(b, j.Relation.JoinPKs[i].SQLName...)
			b = append(b, " = "...)
			b = j.appendBaseAlias(fmter, b)
			b = append(b, '.')
			b = append(b, baseField.SQLName...)
		}
	}

	b = append(b, ')')

	if isSoftDelete {
		b = append(b, " AND "...)
		b = j.appendAlias(fmter, b)
		b = j.appendSoftDelete(fmter, b, q.flags)
	}

	if len(j.additionalJoinOnConditions) > 0 {
		b = append(b, " AND "...)
		b = appendAdditionalJoinOnConditions(fmter, b, j.additionalJoinOnConditions)
	}

	return b, nil
}

func appendChildValues(
	fmter schema.Formatter, b []byte, v reflect.Value, index []int, fields []*schema.Field,
) []byte {
	// Check if any of the fields is an array type
	var hasArrayField bool
	var arrayField *schema.Field
	for _, f := range fields {
		if f.Tag.HasOption("array") {
			hasArrayField = true
			arrayField = f
			break
		}
	}

	// Special handling for PostgreSQL array fields
	if hasArrayField && fmter.Dialect().Name() == dialect.PG {
		// For array fields in PostgreSQL, we need special handling when used directly in WHERE clauses
		// Simple case: a single array field
		if len(fields) == 1 && arrayField != nil {
			// Just return the array value directly - calling function handles ANY() syntax
			return arrayField.AppendValue(fmter, b, reflect.Indirect(v).FieldByIndex(index))
		}
	}

	// Original implementation for non-array relationships
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
// but instead uses old style ((k1=v1) AND (k2=v2)) OR (...) conditions.
func appendMultiValues(
	fmter schema.Formatter, b []byte, v reflect.Value, index []int, baseFields, joinFields []*schema.Field, joinTable schema.Safe,
) []byte {
	// This is based on a mix of appendChildValues and query_base.appendColumns

	// These should never mismatch in length but nice to know if it does
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

func appendAdditionalJoinOnConditions(
	fmter schema.Formatter, b []byte, conditions []schema.QueryWithArgs,
) []byte {
	for i, cond := range conditions {
		if i > 0 {
			b = append(b, " AND "...)
		}
		b = fmter.AppendQuery(b, cond.Query, cond.Args...)
	}
	return b
}

// appendColumnValue is a helper function to append a single column value to the byte buffer
func appendColumnValue(
	fmter schema.Formatter, b []byte, v reflect.Value, index []int, field *schema.Field,
) []byte {
	v = reflect.Indirect(v)
	if len(index) > 0 {
		v = v.FieldByIndex(index)
	}
	return field.AppendValue(fmter, b, v)
}
