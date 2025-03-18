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
		RoleIDs []int64 `bun:",array"`

		// Tagged with array for PostgreSQL array relation
		Roles []Role `bun:",array,rel:has-many,join:role_ids=id"`
	}

	// Test that the array tag is correctly parsed
	userType := reflect.TypeOf(User{})
	rolesField, ok := userType.FieldByName("Roles")
	require.True(t, ok, "Roles field must be found")

	tag := tagparser.Parse(rolesField.Tag.Get("bun"))
	require.True(t, tag.HasOption("array"), "array tag option should be recognized")
	require.Equal(t, "has-many", tag.Options["rel"][0], "relation type should be parsed correctly")

	// Test if the Field.IsArrayRelation() method works correctly
	field := &Field{
		Tag: tag,
	}
	require.True(t, field.IsArrayRelation(), "IsArrayRelation should return true for array-tagged fields")

	// Manually create a Relation to test the IsArray flag
	rel := &Relation{
		Type: HasManyRelation,
		Field: &Field{
			Tag: tag,
		},
		IsArray: field.IsArrayRelation(),
	}

	// Verify that the IsArray flag is set
	require.True(t, rel.IsArray, "IsArray flag should be set on array relations")
}

func TestArrayRelationsTypes(t *testing.T) {
	// Test has-one array relation
	type Post struct {
		ID    int64  `bun:",pk,autoincrement"`
		Title string
	}

	type Author struct {
		ID         int64   `bun:",pk,autoincrement"`
		Name       string
		PostIDs    []int64 `bun:",array"`
		
		// Has-one relation with array
		FeaturedPost *Post `bun:",array,rel:has-one,join:post_ids=id"`
	}

	// Test has-one array relation
	authorType := reflect.TypeOf(Author{})
	postField, ok := authorType.FieldByName("FeaturedPost")
	require.True(t, ok, "FeaturedPost field must be found")

	postTag := tagparser.Parse(postField.Tag.Get("bun"))
	require.True(t, postTag.HasOption("array"), "array tag option should be recognized")
	require.Equal(t, "has-one", postTag.Options["rel"][0], "relation type should be parsed correctly")

	// Test belongs-to array relation
	type Comment struct {
		ID      int64   `bun:",pk,autoincrement"`
		Content string
		PostIDs []int64 `bun:",array"`
		
		// Belongs-to relation with array
		Post    *Post   `bun:",array,rel:belongs-to,join:post_ids=id"`
	}

	commentType := reflect.TypeOf(Comment{})
	commentPostField, ok := commentType.FieldByName("Post")
	require.True(t, ok, "Post field must be found")

	commentPostTag := tagparser.Parse(commentPostField.Tag.Get("bun"))
	require.True(t, commentPostTag.HasOption("array"), "array tag option should be recognized")
	require.Equal(t, "belongs-to", commentPostTag.Options["rel"][0], "relation type should be parsed correctly")
}
