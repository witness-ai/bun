package schema

import (
	"fmt"
)

const (
	InvalidRelation = iota
	HasOneRelation
	BelongsToRelation
	HasManyRelation
	ManyToManyRelation
)

type Relation struct {
	Type       int
	Field      *Field
	JoinTable  *Table
	BaseFields []*Field
	JoinFields []*Field
	OnUpdate   string
	OnDelete   string
	Condition  []string

	PolymorphicField *Field
	PolymorphicValue string

	M2MTable      *Table
	M2MBaseFields []*Field
	M2MJoinFields []*Field

	// IsArrayRelation indicates if this relation is based on array field
	IsArrayRelation bool
}

// NewRelation creates a new relation with specified parameters
func NewRelation(typ int, field *Field, joinTable *Table, isArray bool) *Relation {
	return &Relation{
		Type:            typ,
		Field:           field,
		JoinTable:       joinTable,
		IsArrayRelation: isArray,
	}
}

// References returns true if the table to which the Relation belongs needs to declare a foreign key constraint to create the relation.
// For other relations, the constraint is created in either the referencing table (1:N, 'has-many' relations) or a mapping table (N:N, 'm2m' relations).
func (r *Relation) References() bool {
	return r.Type == HasOneRelation || r.Type == BelongsToRelation
}

// IsArray returns true if the relation is based on an array field
func (r *Relation) IsArray() bool {
	result := r.IsArrayRelation || (r.Field != nil && r.Field.Tag.HasOption("array"))

	// Print debug info about array relation detection
	fmt.Printf("DEBUG IsArray: IsArrayRelation=%v, HasArrayOption=%v, result=%v\n",
		r.IsArrayRelation,
		r.Field != nil && r.Field.Tag.HasOption("array"),
		result)

	return result
}

func (r *Relation) String() string {
	return fmt.Sprintf("relation=%s", r.Field.GoName)
}
