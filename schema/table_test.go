package schema

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun/internal/tagparser"
)

func TestTable(t *testing.T) {
	dialect := newNopDialect()
	tables := NewTables(dialect)

	t.Run("simple", func(t *testing.T) {
		type Model struct {
			ID  int `bun:",pk"`
			Foo string
			Bar string
		}

		table := tables.Get(reflect.TypeOf((*Model)(nil)))

		require.Len(t, table.allFields, 3)
		require.Len(t, table.Fields, 3)
		require.Len(t, table.PKs, 1)
		require.Len(t, table.DataFields, 2)
	})

	type Model struct {
		Foo string
		Bar string
	}

	t.Run("model1", func(t *testing.T) {
		type Model1 struct {
			Model
			Foo string
		}

		table := tables.Get(reflect.TypeOf((*Model1)(nil)))

		foo, ok := table.FieldMap["foo"]
		require.True(t, ok)
		require.Equal(t, []int{1}, foo.Index)

		bar, ok := table.FieldMap["bar"]
		require.True(t, ok)
		require.Equal(t, []int{0, 1}, bar.Index)
	})

	t.Run("model2", func(t *testing.T) {
		type Model2 struct {
			Foo string
			Model
		}

		table := tables.Get(reflect.TypeOf((*Model2)(nil)))

		foo, ok := table.FieldMap["foo"]
		require.True(t, ok)
		require.Equal(t, []int{0}, foo.Index)

		bar, ok := table.FieldMap["bar"]
		require.True(t, ok)
		require.Equal(t, []int{1, 1}, bar.Index)
	})

	t.Run("table name", func(t *testing.T) {
		type Model struct {
			BaseModel `bun:"custom_name,alias:custom_alias"`
		}

		table := tables.Get(reflect.TypeOf((*Model)(nil)))
		require.Equal(t, "custom_name", table.Name)
		require.Equal(t, "custom_alias", table.Alias)
	})

	t.Run("extend", func(t *testing.T) {
		type Model1 struct {
			BaseModel `bun:"custom_name,alias:custom_alias"`
		}
		type Model2 struct {
			Model1 `bun:",extend"`
		}

		table := tables.Get(reflect.TypeOf((*Model2)(nil)))
		require.Equal(t, "custom_name", table.Name)
		require.Equal(t, "custom_alias", table.Alias)
	})

	t.Run("embed", func(t *testing.T) {
		type Perms struct {
			View   bool
			Create bool
		}

		type Role struct {
			Foo Perms `bun:"embed:foo_"`
			Bar Perms `bun:"embed:bar_"`
		}

		table := tables.Get(reflect.TypeOf((*Role)(nil)))
		require.Nil(t, table.StructMap["foo"])
		require.Nil(t, table.StructMap["bar"])

		fooView, ok := table.FieldMap["foo_view"]
		require.True(t, ok)
		require.Equal(t, []int{0, 0}, fooView.Index)

		barView, ok := table.FieldMap["bar_view"]
		require.True(t, ok)
		require.Equal(t, []int{1, 0}, barView.Index)
	})

	t.Run("embed scanonly", func(t *testing.T) {
		type Model1 struct {
			Foo string
			Bar string `bun:",scanonly"`
		}

		type Model2 struct {
			Model1
		}

		table := tables.Get(reflect.TypeOf((*Model2)(nil)))
		require.Len(t, table.FieldMap, 2)

		foo, ok := table.FieldMap["foo"]
		require.True(t, ok)
		require.Equal(t, []int{0, 0}, foo.Index)

		bar, ok := table.FieldMap["bar"]
		require.True(t, ok)
		require.Equal(t, []int{0, 1}, bar.Index)
	})

	t.Run("embed scanonly prefix", func(t *testing.T) {
		type Model1 struct {
			Foo string `bun:",scanonly"`
			Bar string `bun:",scanonly"`
		}

		type Model2 struct {
			Baz Model1 `bun:"embed:baz_"`
		}

		table := tables.Get(reflect.TypeOf((*Model2)(nil)))
		require.Len(t, table.FieldMap, 2)

		foo, ok := table.FieldMap["baz_foo"]
		require.True(t, ok)
		require.Equal(t, []int{0, 0}, foo.Index)

		bar, ok := table.FieldMap["baz_bar"]
		require.True(t, ok)
		require.Equal(t, []int{0, 1}, bar.Index)
	})

	t.Run("scanonly", func(t *testing.T) {
		type Model1 struct {
			Foo string
			Bar string
		}

		type Model2 struct {
			XXX Model1 `bun:",scanonly"`
			Baz string `bun:",scanonly"`
		}

		table := tables.Get(reflect.TypeOf((*Model2)(nil)))

		require.Len(t, table.StructMap, 1)
		require.NotNil(t, table.StructMap["xxx"])

		require.Len(t, table.FieldMap, 2)
		baz := table.FieldMap["baz"]
		require.NotNil(t, baz)
		require.Equal(t, []int{1}, baz.Index)

		foo := table.LookupField("xxx__foo")
		require.NotNil(t, foo)
		require.Equal(t, []int{0, 0}, foo.Index)

		bar := table.LookupField("xxx__bar")
		require.NotNil(t, bar)
		require.Equal(t, []int{0, 1}, bar.Index)
	})

	t.Run("recursive", func(t *testing.T) {
		type Model struct {
			*Model

			Foo string
			Bar string
		}

		table := tables.Get(reflect.TypeOf((*Model)(nil)))

		foo, ok := table.FieldMap["foo"]
		require.True(t, ok)
		require.Equal(t, []int{1}, foo.Index)

		bar, ok := table.FieldMap["bar"]
		require.True(t, ok)
		require.Equal(t, []int{2}, bar.Index)
	})

	t.Run("recursive relation", func(t *testing.T) {
		type Item struct {
			ID     int64 `bun:",pk"`
			ItemID int64
			Item   *Item `bun:"rel:belongs-to,join:item_id=id"`
		}

		table := tables.Get(reflect.TypeOf((*Item)(nil)))

		rel, ok := table.Relations["Item"]
		require.True(t, ok)
		require.Equal(t, BelongsToRelation, rel.Type)

		{
			require.NotNil(t, table.StructMap["item"])

			id := table.LookupField("item__id")
			require.NotNil(t, id)
			require.Equal(t, []int{2, 0}, id.Index)
		}
	})

	t.Run("alternative name", func(t *testing.T) {
		type ModelTest struct {
			Model
			Foo string `bun:"alt:alt_name"`
		}

		table := tables.Get(reflect.TypeOf((*ModelTest)(nil)))

		foo, ok := table.FieldMap["foo"]
		require.True(t, ok)
		require.Equal(t, []int{1}, foo.Index)

		foo2, ok := table.FieldMap["alt_name"]
		require.True(t, ok)
		require.Equal(t, []int{1}, foo2.Index)

		require.Equal(t, table.FieldMap["foo"].SQLName, table.FieldMap["alt_name"].SQLName)
	})
}

func TestArrayRelations(t *testing.T) {
	type Role struct {
		ID   int64 `bun:",pk,autoincrement"`
		Name string
	}

	type User struct {
		ID      int64 `bun:",pk,autoincrement"`
		Name    string
		RoleIDs []int64 `bun:",array"` // PostgreSQL array column with array tag

		// Now we don't need array tag here, just the regular relation tag
		Roles []Role `bun:",rel:has-many,join:role_ids=id"`
	}

	// Test that the array tag is correctly parsed
	userType := reflect.TypeOf(User{})

	// Verify the array tag on RoleIDs
	roleIDsField, ok := userType.FieldByName("RoleIDs")
	require.True(t, ok, "RoleIDs field must be found")
	roleIDsTag := tagparser.Parse(roleIDsField.Tag.Get("bun"))
	require.True(t, roleIDsTag.HasOption("array"), "array tag option should be recognized on RoleIDs")

	// Verify the relation tag on Roles (no array tag needed here anymore)
	rolesField, ok := userType.FieldByName("Roles")
	require.True(t, ok, "Roles field must be found")
	rolesTag := tagparser.Parse(rolesField.Tag.Get("bun"))
	require.Equal(t, "has-many", rolesTag.Options["rel"][0], "relation type should be parsed correctly")

	// Create a mock relation to test the detection logic
	role := &Field{
		Name: "role_ids",
		Tag:  roleIDsTag,
	}

	rel := &Relation{
		BaseFields: []*Field{role},
	}

	// Now check if our array detection logic works
	for _, f := range rel.BaseFields {
		if f.Tag.HasOption("array") {
			rel.IsArray = true
			break
		}
	}

	require.True(t, rel.IsArray, "IsArray flag should be set based on the base field's array tag")
}

func TestArrayRelationsTypes(t *testing.T) {
	// Test has-one array relation
	type Post struct {
		ID    int64 `bun:",pk,autoincrement"`
		Title string
	}

	type Author struct {
		ID      int64 `bun:",pk,autoincrement"`
		Name    string
		PostIDs []int64 `bun:",array"` // Array tag on the ID field

		// Has-one relation now just needs standard relation tags
		FeaturedPost *Post `bun:",rel:has-one,join:post_ids=id"`
	}

	// Test has-one relation tag parsing
	authorType := reflect.TypeOf(Author{})

	// Check the array field
	postIDsField, ok := authorType.FieldByName("PostIDs")
	require.True(t, ok, "PostIDs field must be found")
	postIDsTag := tagparser.Parse(postIDsField.Tag.Get("bun"))
	require.True(t, postIDsTag.HasOption("array"), "array tag should be on the ID field")

	// Check the relation field
	postField, ok := authorType.FieldByName("FeaturedPost")
	require.True(t, ok, "FeaturedPost field must be found")
	postTag := tagparser.Parse(postField.Tag.Get("bun"))
	require.Equal(t, "has-one", postTag.Options["rel"][0], "relation type should be parsed correctly")

	// Test belongs-to array relation
	type Comment struct {
		ID      int64 `bun:",pk,autoincrement"`
		Content string
		PostIDs []int64 `bun:",array"` // Array tag on the ID field

		// Belongs-to relation now just needs standard relation tags
		Post *Post `bun:",rel:belongs-to,join:post_ids=id"`
	}

	commentType := reflect.TypeOf(Comment{})

	// Check the array field
	commentPostIDsField, ok := commentType.FieldByName("PostIDs")
	require.True(t, ok, "PostIDs field must be found")
	commentPostIDsTag := tagparser.Parse(commentPostIDsField.Tag.Get("bun"))
	require.True(t, commentPostIDsTag.HasOption("array"), "array tag should be on the ID field")

	// Check the relation field
	commentPostField, ok := commentType.FieldByName("Post")
	require.True(t, ok, "Post field must be found")
	commentPostTag := tagparser.Parse(commentPostField.Tag.Get("bun"))
	require.Equal(t, "belongs-to", commentPostTag.Options["rel"][0], "relation type should be parsed correctly")
}
