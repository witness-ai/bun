package dbtest_test

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dbfixture"
	"github.com/uptrace/bun/dialect"
	"github.com/uptrace/bun/dialect/feature"
)

func TestORM(t *testing.T) {
	type Test struct {
		fn func(*testing.T, *bun.DB)
	}

	tests := []Test{
		{testBookRelations},
		{testAuthorRelations},
		{testGenreRelations},
		{testTranslationRelations},
		{testBulkUpdate},
		{testRelationColumn},
		{testRelationExcludeAll},
		{testM2MRelationExcludeColumn},
		{testRelationBelongsToSelf},
		{testCompositeHasMany},
		{testArrayRelations},
		{testArrayM2MRelations},
	}

	testEachDB(t, func(t *testing.T, dbName string, db *bun.DB) {
		createTestSchema(t, db)

		for _, test := range tests {
			loadTestData(t, ctx, db)

			t.Run(funcName(test.fn), func(t *testing.T) {
				test.fn(t, db)
			})
		}
	})
}

func testBookRelations(t *testing.T, db *bun.DB) {
	book := new(Book)
	err := db.NewSelect().
		Model(book).
		Column("book.id").
		Relation("Author").
		Relation("Author.Avatar").
		Relation("Editor").
		Relation("Editor.Avatar").
		Relation("Genres", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Column("id", "name", "genre__rating")
		}).
		Relation("Comments").
		Relation("Translations", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Order("id")
		}).
		Relation("Translations.Comments", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Order("text")
		}).
		OrderExpr("book.id ASC").
		Limit(1).
		Scan(ctx)
	require.NoError(t, err)
	require.Equal(t, 100, book.ID)
	require.Equal(t, &Book{
		ID:    100,
		Title: "",
		Author: Author{
			ID:       10,
			Name:     "author 1",
			AvatarID: 1,
			Avatar: Image{
				ID:   1,
				Path: "/path/to/1.jpg",
			},
		},
		Editor: &Author{
			ID:       11,
			Name:     "author 2",
			AvatarID: 2,
			Avatar: Image{
				ID:   2,
				Path: "/path/to/2.jpg",
			},
		},
		CreatedAt: time.Time{},
		Genres: []Genre{
			{ID: 1, Name: "genre 1", Rating: 999},
			{ID: 2, Name: "genre 2", Rating: 9999},
		},
		Translations: []Translation{{
			ID:     1000,
			BookID: 100,
			Lang:   "ru",
			Comments: []Comment{
				{TrackableID: 1000, TrackableType: "translation", Text: "comment3"},
			},
		}, {
			ID:       1001,
			BookID:   100,
			Lang:     "md",
			Comments: nil,
		}},
		Comments: []Comment{
			{TrackableID: 100, TrackableType: "book", Text: "comment1"},
			{TrackableID: 100, TrackableType: "book", Text: "comment2"},
		},
	}, book)
}

func testAuthorRelations(t *testing.T, db *bun.DB) {
	var author Author
	err := db.NewSelect().
		Model(&author).
		Column("author.*").
		Relation("Books", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Column("book.id", "book.author_id", "book.editor_id").OrderExpr("book.id ASC")
		}).
		Relation("Books.Author").
		Relation("Books.Editor").
		Relation("Books.Translations", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.OrderExpr("tr.id ASC")
		}).
		OrderExpr("author.id ASC").
		Limit(1).
		Scan(ctx)
	require.NoError(t, err)
	require.Equal(t, Author{
		ID:       10,
		Name:     "author 1",
		AvatarID: 1,
		Books: []*Book{{
			ID:        100,
			Title:     "",
			AuthorID:  10,
			Author:    Author{ID: 10, Name: "author 1", AvatarID: 1},
			EditorID:  11,
			Editor:    &Author{ID: 11, Name: "author 2", AvatarID: 2},
			CreatedAt: time.Time{},
			Genres:    nil,
			Translations: []Translation{
				{ID: 1000, BookID: 100, Book: nil, Lang: "ru", Comments: nil},
				{ID: 1001, BookID: 100, Book: nil, Lang: "md", Comments: nil},
			},
		}, {
			ID:        101,
			Title:     "",
			AuthorID:  10,
			Author:    Author{ID: 10, Name: "author 1", AvatarID: 1},
			EditorID:  12,
			Editor:    &Author{ID: 12, Name: "author 3", AvatarID: 3},
			CreatedAt: time.Time{},
			Genres:    nil,
			Translations: []Translation{
				{ID: 1002, BookID: 101, Book: nil, Lang: "ua", Comments: nil},
			},
		}},
	}, author)
}

func testGenreRelations(t *testing.T, db *bun.DB) {
	var genre Genre
	err := db.NewSelect().
		Model(&genre).
		Column("genre.*").
		Relation("Books", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.ColumnExpr("book.id")
		}).
		Relation("Books.Translations", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.OrderExpr("tr.id ASC")
		}).
		OrderExpr("genre.id ASC").
		Limit(1).
		Scan(ctx)
	require.NoError(t, err)
	require.Equal(t, Genre{
		ID:     1,
		Name:   "genre 1",
		Rating: 0,
		Books: []Book{{
			ID: 100,
			Translations: []Translation{
				{ID: 1000, BookID: 100, Book: nil, Lang: "ru", Comments: nil},
				{ID: 1001, BookID: 100, Book: nil, Lang: "md", Comments: nil},
			},
		}, {
			ID: 101,
			Translations: []Translation{
				{ID: 1002, BookID: 101, Book: nil, Lang: "ua", Comments: nil},
			},
		}},
		ParentID:  0,
		Subgenres: nil,
	}, genre)
}

func testTranslationRelations(t *testing.T, db *bun.DB) {
	var translation Translation
	err := db.NewSelect().
		Model(&translation).
		Column("tr.*").
		Relation("Book", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.ColumnExpr("book.id AS book__id")
		}).
		Relation("Book.Author").
		Relation("Book.Editor").
		OrderExpr("tr.id ASC").
		Limit(1).
		Scan(ctx)
	require.NoError(t, err)
	require.Equal(t, Translation{
		ID:     1000,
		BookID: 100,
		Book: &Book{
			ID:     100,
			Author: Author{ID: 10, Name: "author 1", AvatarID: 1},
			Editor: &Author{ID: 11, Name: "author 2", AvatarID: 2},
		},
		Lang: "ru",
	}, translation)
}

func testBulkUpdate(t *testing.T, db *bun.DB) {
	if !db.Dialect().Features().Has(feature.CTE) {
		t.Skip()
	}

	var books []Book
	err := db.NewSelect().Model(&books).Scan(ctx)
	require.NoError(t, err)

	res, err := db.NewUpdate().
		With("_data", db.NewValues(&books)).
		Model((*Book)(nil)).
		Table("_data").
		Apply(func(q *bun.UpdateQuery) *bun.UpdateQuery {
			return q.
				SetColumn("title", "UPPER(?)", q.FQN("title")).
				Where("? = _data.id", q.FQN("id"))
		}).
		Exec(ctx)
	require.NoError(t, err)

	n, err := res.RowsAffected()
	require.NoError(t, err)
	require.Equal(t, len(books), int(n))

	var books2 []Book
	err = db.NewSelect().Model(&books2).Scan(ctx)
	require.NoError(t, err)

	for i := range books {
		require.Equal(t, strings.ToUpper(books[i].Title), books2[i].Title)
	}
}

func testRelationColumn(t *testing.T, db *bun.DB) {
	book := new(Book)
	err := db.NewSelect().
		Model(book).
		ExcludeColumn("created_at").
		Relation("Author", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Column("name")
		}).
		OrderExpr("book.id").
		Limit(1).
		Scan(ctx)
	require.NoError(t, err)
	require.Equal(t, &Book{
		ID:       100,
		Title:    "book 1",
		AuthorID: 10,
		Author: Author{
			Name: "author 1",
		},
		EditorID: 11,
	}, book)
}

func testRelationExcludeAll(t *testing.T, db *bun.DB) {
	book := new(Book)
	err := db.NewSelect().
		Model(book).
		ExcludeColumn("created_at").
		Relation("Author", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.ExcludeColumn("*")
		}).
		Relation("Author.Avatar").
		Relation("Editor").
		OrderExpr("book.id").
		Limit(1).
		Scan(ctx)
	require.NoError(t, err)
	require.Equal(t, &Book{
		ID:       100,
		Title:    "book 1",
		AuthorID: 10,
		Author: Author{
			Avatar: Image{
				ID:   1,
				Path: "/path/to/1.jpg",
			},
		},
		EditorID: 11,
		Editor: &Author{
			ID:       11,
			Name:     "author 2",
			AvatarID: 2,
		},
	}, book)

	book = new(Book)
	err = db.NewSelect().
		Model(book).
		ExcludeColumn("*").
		Relation("Author", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.ExcludeColumn("*")
		}).
		Relation("Author.Avatar").
		Relation("Editor").
		OrderExpr("book.id").
		Limit(1).
		Scan(ctx)
	require.NoError(t, err)
	require.Equal(t, &Book{
		Author: Author{
			Avatar: Image{
				ID:   1,
				Path: "/path/to/1.jpg",
			},
		},
		Editor: &Author{
			ID:       11,
			Name:     "author 2",
			AvatarID: 2,
		},
	}, book)
}

func testRelationBelongsToSelf(t *testing.T, db *bun.DB) {
	type Model struct {
		bun.BaseModel `bun:"alias:m"`

		ID      int64 `bun:",pk,autoincrement"`
		ModelID int64
		Model   *Model `bun:"rel:belongs-to"`
	}

	mustResetModel(t, ctx, db, (*Model)(nil))

	models := []Model{
		{ID: 1},
		{ID: 2, ModelID: 1},
	}
	_, err := db.NewInsert().Model(&models).Exec(ctx)
	require.NoError(t, err)

	models = nil
	err = db.NewSelect().Model(&models).Relation("Model").OrderExpr("m.id ASC").Scan(ctx)
	require.NoError(t, err)
	require.Equal(t, []Model{
		{ID: 1},
		{ID: 2, ModelID: 1, Model: &Model{ID: 1}},
	}, models)
}

func testM2MRelationExcludeColumn(t *testing.T, db *bun.DB) {
	type Item struct {
		ID        int64     `bun:",pk,autoincrement"`
		CreatedAt time.Time `bun:",notnull,nullzero"`
		UpdatedAt time.Time `bun:",notnull,nullzero"`
	}

	type Order struct {
		ID    int64 `bun:",pk,autoincrement"`
		Text  string
		Items []Item `bun:"m2m:order_to_items"`
	}

	type OrderToItem struct {
		OrderID   int64     `bun:",pk"`
		Order     *Order    `bun:"rel:has-one,join:order_id=id"`
		ItemID    int64     `bun:",pk"`
		Item      *Item     `bun:"rel:has-one,join:item_id=id"`
		CreatedAt time.Time `bun:",notnull,nullzero"`
	}

	db.RegisterModel((*OrderToItem)(nil))
	mustResetModel(t, ctx, db, (*Order)(nil), (*Item)(nil), (*OrderToItem)(nil))

	items := []Item{
		{ID: 1, CreatedAt: time.Unix(1, 0), UpdatedAt: time.Unix(1, 0)},
		{ID: 2, CreatedAt: time.Unix(2, 0), UpdatedAt: time.Unix(1, 0)},
	}
	_, err := db.NewInsert().Model(&items).Exec(ctx)
	require.NoError(t, err)

	orders := []Order{
		{ID: 1},
		{ID: 2},
	}
	_, err = db.NewInsert().Model(&orders).Exec(ctx)
	require.NoError(t, err)

	orderItems := []OrderToItem{
		{OrderID: 1, ItemID: 1, CreatedAt: time.Unix(3, 0)},
		{OrderID: 2, ItemID: 2, CreatedAt: time.Unix(4, 0)},
	}
	_, err = db.NewInsert().Model(&orderItems).Exec(ctx)
	require.NoError(t, err)

	order := new(Order)
	err = db.NewSelect().
		Model(order).
		Where("id = ?", 1).
		Relation("Items", func(sq *bun.SelectQuery) *bun.SelectQuery {
			return sq.ExcludeColumn("updated_at")
		}).
		Scan(ctx)
	require.NoError(t, err)
}

func testCompositeHasMany(t *testing.T, db *bun.DB) {
	department := new(Department)
	err := db.NewSelect().
		Model(department).
		Where("company_no=? AND no=?", "company one", "hr").
		Relation("Employees").
		Scan(ctx)
	require.NoError(t, err)
	require.Equal(t, "hr", department.No)
	require.Equal(t, 2, len(department.Employees))
}

func testArrayRelations(t *testing.T, db *bun.DB) {
	// Skip test for non-PostgreSQL dialects
	if db.Dialect().Name() != dialect.PG {
		t.Skip("array relations are only supported in PostgreSQL")
	}

	// Define models with array relation
	type Role struct {
		ID   int64 `bun:",pk,autoincrement"`
		Name string
	}

	type User struct {
		ID      int64 `bun:",pk,autoincrement"`
		Name    string
		RoleIDs []int64 `bun:",array"` // PostgreSQL array column

		// Define a relation that uses the array column
		Roles []Role `bun:",array,rel:has-many,join:role_ids=id"`
	}

	// Create schema for test
	_, err := db.NewCreateTable().Model((*Role)(nil)).IfNotExists().Exec(ctx)
	require.NoError(t, err)

	_, err = db.NewCreateTable().Model((*User)(nil)).IfNotExists().Exec(ctx)
	require.NoError(t, err)

	// Insert test data
	roles := []Role{
		{ID: 1, Name: "Admin"},
		{ID: 2, Name: "Moderator"},
		{ID: 3, Name: "User"},
	}
	_, err = db.NewInsert().Model(&roles).Exec(ctx)
	require.NoError(t, err)

	users := []User{
		{ID: 1, Name: "John", RoleIDs: []int64{1, 2}},     // Admin, Moderator
		{ID: 2, Name: "Jane", RoleIDs: []int64{2, 3}},     // Moderator, User
		{ID: 3, Name: "Bob", RoleIDs: []int64{3}},         // User only
		{ID: 4, Name: "Alice", RoleIDs: []int64{1, 2, 3}}, // All roles
	}
	_, err = db.NewInsert().Model(&users).Exec(ctx)
	require.NoError(t, err)

	// Test 1: Query user with roles via array relation
	var user User
	err = db.NewSelect().
		Model(&user).
		Where("id = ?", 1).
		Relation("Roles").
		Scan(ctx)

	require.NoError(t, err)
	require.Equal(t, "John", user.Name)
	require.Len(t, user.Roles, 2, "Should have loaded 2 roles")

	// Check that we got the right roles
	roleNames := []string{user.Roles[0].Name, user.Roles[1].Name}
	require.Contains(t, roleNames, "Admin")
	require.Contains(t, roleNames, "Moderator")

	// Test 2: Query all users with roles
	var allUsers []User
	err = db.NewSelect().
		Model(&allUsers).
		Relation("Roles").
		Order("id ASC").
		Scan(ctx)

	require.NoError(t, err)
	require.Len(t, allUsers, 4)

	// Verify first user's roles
	require.Len(t, allUsers[0].Roles, 2)

	// Verify second user's roles
	require.Len(t, allUsers[1].Roles, 2)

	// Verify third user's roles
	require.Len(t, allUsers[2].Roles, 1)
	require.Equal(t, "User", allUsers[2].Roles[0].Name)

	// Verify fourth user's roles
	require.Len(t, allUsers[3].Roles, 3)

	// Test 3: Query users with a specific role (using WHERE EXISTS)
	var adminUsers []User
	err = db.NewSelect().
		Model(&adminUsers).
		Where("1 = ANY(role_ids)"). // Users with Admin role (ID 1)
		Scan(ctx)

	require.NoError(t, err)
	require.Len(t, adminUsers, 2) // John and Alice

	// Test 4: Define models for has-one and belongs-to with array relations
	type Post struct {
		ID       int64 `bun:",pk,autoincrement"`
		Title    string
		AuthorID int64
	}

	type Comment struct {
		ID      int64 `bun:",pk,autoincrement"`
		Content string
		PostIDs []int64 `bun:",array"` // Array of post IDs

		// A comment belongs to one of multiple possible posts (array-based belongs-to)
		Post *Post `bun:",array,rel:belongs-to,join:post_ids=id"`
	}

	type Author struct {
		ID      int64 `bun:",pk,autoincrement"`
		Name    string
		PostIDs []int64 `bun:",array"` // Array of post IDs

		// Author has one featured post stored in an array (array-based has-one)
		FeaturedPost *Post `bun:",array,rel:has-one,join:post_ids=id"`
	}

	// Create schema
	_, err = db.NewCreateTable().Model((*Post)(nil)).IfNotExists().Exec(ctx)
	require.NoError(t, err)

	_, err = db.NewCreateTable().Model((*Comment)(nil)).IfNotExists().Exec(ctx)
	require.NoError(t, err)

	_, err = db.NewCreateTable().Model((*Author)(nil)).IfNotExists().Exec(ctx)
	require.NoError(t, err)

	// Insert test data
	posts := []Post{
		{ID: 1, Title: "PostgreSQL Arrays", AuthorID: 1},
		{ID: 2, Title: "Go Programming", AuthorID: 1},
		{ID: 3, Title: "Database Design", AuthorID: 2},
	}
	_, err = db.NewInsert().Model(&posts).Exec(ctx)
	require.NoError(t, err)

	authors := []Author{
		{ID: 1, Name: "John Doe", PostIDs: []int64{1, 2}}, // Has posts 1 and 2
		{ID: 2, Name: "Jane Smith", PostIDs: []int64{3}},  // Has post 3
	}
	_, err = db.NewInsert().Model(&authors).Exec(ctx)
	require.NoError(t, err)

	comments := []Comment{
		{ID: 1, Content: "Great article!", PostIDs: []int64{1}},    // Belongs to post 1
		{ID: 2, Content: "Interesting topic", PostIDs: []int64{2}}, // Belongs to post 2
		{ID: 3, Content: "Multiple posts", PostIDs: []int64{1, 3}}, // Belongs to posts 1 and 3
	}
	_, err = db.NewInsert().Model(&comments).Exec(ctx)
	require.NoError(t, err)

	// Test has-one with array: Get author with featured post
	var author Author
	err = db.NewSelect().
		Model(&author).
		Where("id = ?", 1).
		Relation("FeaturedPost").
		Scan(ctx)

	require.NoError(t, err)
	require.NotNil(t, author.FeaturedPost)
	require.Equal(t, "PostgreSQL Arrays", author.FeaturedPost.Title)

	// Test belongs-to with array: Get comment with post
	var comment Comment
	err = db.NewSelect().
		Model(&comment).
		Where("id = ?", 1).
		Relation("Post").
		Scan(ctx)

	require.NoError(t, err)
	require.NotNil(t, comment.Post)
	require.Equal(t, "PostgreSQL Arrays", comment.Post.Title)

	// Test 5: Test empty array case
	emptyAuthor := Author{
		ID:      5,
		Name:    "Empty Array Author",
		PostIDs: []int64{}, // Empty array
	}
	_, err = db.NewInsert().Model(&emptyAuthor).Exec(ctx)
	require.NoError(t, err)

	// Load empty array author with relation
	var emptyArrayAuthor Author
	err = db.NewSelect().
		Model(&emptyArrayAuthor).
		Where("id = ?", 5).
		Relation("FeaturedPost").
		Scan(ctx)

	require.NoError(t, err)
	require.Nil(t, emptyArrayAuthor.FeaturedPost, "Featured post should be nil for empty array")

	// Test 6: Test SQL generation with ANY operator
	// We can't directly check the SQL, but we can check it works correctly
	// by querying based on array contents
	type PostWithUser struct {
		ID     int64 `bun:",pk,autoincrement"`
		Title  string
		UserID int64

		// User relation
		User *User
	}

	// Create tables
	_, err = db.NewCreateTable().Model((*PostWithUser)(nil)).IfNotExists().Exec(ctx)
	require.NoError(t, err)

	// Insert data
	postsWithUsers := []PostWithUser{
		{ID: 10, Title: "Post for User 1", UserID: 1},
		{ID: 11, Title: "Post for User 2", UserID: 2},
		{ID: 12, Title: "Post for User 3", UserID: 3},
	}
	_, err = db.NewInsert().Model(&postsWithUsers).Exec(ctx)
	require.NoError(t, err)

	// Query using SQL ANY in a WHERE clause to verify it works correctly
	var usersWithPosts []User
	err = db.NewSelect().
		Model(&usersWithPosts).
		Where("id = ANY(?)", []int64{1, 3}). // Using ANY directly in SQL
		Scan(ctx)

	require.NoError(t, err)
	require.Len(t, usersWithPosts, 2, "Should find 2 users")
	require.Equal(t, "John", usersWithPosts[0].Name, "First user should be John")
	require.Equal(t, "Bob", usersWithPosts[1].Name, "Second user should be Bob")

	// Test 7: NULL array handling
	// Create a user with NULL array (we'll use a direct SQL query for this)
	_, err = db.Exec("INSERT INTO authors (id, name) VALUES (6, 'Null Array Author')")
	require.NoError(t, err)

	// Query the null array author
	var nullArrayAuthor Author
	err = db.NewSelect().
		Model(&nullArrayAuthor).
		Where("id = ?", 6).
		Relation("FeaturedPost").
		Scan(ctx)

	require.NoError(t, err)
	require.Nil(t, nullArrayAuthor.FeaturedPost, "Featured post should be nil for NULL array")
	require.Empty(t, nullArrayAuthor.PostIDs, "PostIDs should be empty for NULL array")
}

func testArrayM2MRelations(t *testing.T, db *bun.DB) {
	// Skip test for non-PostgreSQL dialects
	if db.Dialect().Name() != dialect.PG {
		t.Skip("array relations are only supported in PostgreSQL")
	}

	// Define models with array relation simulating many-to-many
	type Tag struct {
		ID   int64 `bun:",pk,autoincrement"`
		Name string
	}

	type Article struct {
		ID     int64 `bun:",pk,autoincrement"`
		Title  string
		TagIDs []int64 `bun:",array"` // PostgreSQL array column instead of join table

		// Define an array-based relation that replaces traditional M2M
		Tags []Tag `bun:",array,rel:has-many,join:tag_ids=id"`
	}

	// Create schema for test
	_, err := db.NewCreateTable().Model((*Tag)(nil)).IfNotExists().Exec(ctx)
	require.NoError(t, err)

	_, err = db.NewCreateTable().Model((*Article)(nil)).IfNotExists().Exec(ctx)
	require.NoError(t, err)

	// Insert test data
	tags := []Tag{
		{ID: 1, Name: "Technology"},
		{ID: 2, Name: "Programming"},
		{ID: 3, Name: "Database"},
		{ID: 4, Name: "Go"},
		{ID: 5, Name: "PostgreSQL"},
	}
	_, err = db.NewInsert().Model(&tags).Exec(ctx)
	require.NoError(t, err)

	articles := []Article{
		{ID: 1, Title: "Go Programming", TagIDs: []int64{1, 2, 4}},
		{ID: 2, Title: "PostgreSQL Arrays", TagIDs: []int64{1, 3, 5}},
		{ID: 3, Title: "Go with PostgreSQL", TagIDs: []int64{1, 2, 3, 4, 5}},
	}
	_, err = db.NewInsert().Model(&articles).Exec(ctx)
	require.NoError(t, err)

	// Test 1: Query articles with their tags
	var articles1 []Article
	err = db.NewSelect().
		Model(&articles1).
		Relation("Tags").
		Order("id ASC").
		Scan(ctx)

	require.NoError(t, err)
	require.Len(t, articles1, 3)

	// Verify the first article's tags
	require.Len(t, articles1[0].Tags, 3)
	tagNames := make([]string, len(articles1[0].Tags))
	for i, tag := range articles1[0].Tags {
		tagNames[i] = tag.Name
	}
	require.Contains(t, tagNames, "Technology")
	require.Contains(t, tagNames, "Programming")
	require.Contains(t, tagNames, "Go")

	// Verify the third article has all tags
	require.Len(t, articles1[2].Tags, 5)

	// Test 2: Query a specific article with tags
	var article Article
	err = db.NewSelect().
		Model(&article).
		Where("id = ?", 2).
		Relation("Tags").
		Scan(ctx)

	require.NoError(t, err)
	require.Equal(t, "PostgreSQL Arrays", article.Title)
	require.Len(t, article.Tags, 3)

	// Check specific tags
	var hasPostgresTag bool
	for _, tag := range article.Tags {
		if tag.Name == "PostgreSQL" {
			hasPostgresTag = true
			break
		}
	}
	require.True(t, hasPostgresTag, "Article should have PostgreSQL tag")

	// Test 3: Find articles with a specific tag using the ANY operator
	var postgresArticles []Article
	err = db.NewSelect().
		Model(&postgresArticles).
		Where("5 = ANY(tag_ids)"). // Articles with PostgreSQL tag (ID 5)
		Order("id ASC").
		Scan(ctx)

	require.NoError(t, err)
	require.Len(t, postgresArticles, 2) // Should find 2 articles
	require.Equal(t, "PostgreSQL Arrays", postgresArticles[0].Title)
	require.Equal(t, "Go with PostgreSQL", postgresArticles[1].Title)
}

type Genre struct {
	ID     int `bun:",pk"`
	Name   string
	Rating int `bun:",scanonly"`

	Books []Book `bun:"m2m:book_genres"`

	ParentID  int
	Subgenres []Genre `bun:"rel:has-many,join:id=parent_id"`
}

func (g Genre) String() string {
	return fmt.Sprintf("Genre<Id=%d Name=%q>", g.ID, g.Name)
}

type Image struct {
	ID   int `bun:",pk"`
	Path string
}

type Author struct {
	ID    int     `bun:",pk"`
	Name  string  `bun:",unique"`
	Books []*Book `bun:"rel:has-many"`

	AvatarID int
	Avatar   Image `bun:"rel:belongs-to"`
}

func (a Author) String() string {
	return fmt.Sprintf("Author<ID=%d Name=%q>", a.ID, a.Name)
}

var _ bun.BeforeAppendModelHook = (*Author)(nil)

func (*Author) BeforeAppendModel(ctx context.Context, query bun.Query) error {
	return nil
}

type BookGenre struct {
	bun.BaseModel `bun:"alias:bg"` // custom table alias

	BookID  int    `bun:",pk"`
	Book    *Book  `bun:"rel:belongs-to"`
	GenreID int    `bun:",pk"`
	Genre   *Genre `bun:"rel:belongs-to"`

	Genre_Rating int // is copied to Genre.Rating
}

type Book struct {
	ID        int `bun:",pk"`
	Title     string
	AuthorID  int
	Author    Author `bun:"rel:belongs-to"`
	EditorID  int
	Editor    *Author   `bun:"rel:belongs-to"`
	CreatedAt time.Time `bun:",nullzero,default:current_timestamp"`
	UpdatedAt time.Time `bun:",nullzero"`

	Genres       []Genre       `bun:"m2m:book_genres"` // many to many relation
	Translations []Translation `bun:"rel:has-many"`
	Comments     []Comment     `bun:"rel:has-many,join:id=trackable_id,join:type=trackable_type,polymorphic"`
}

func (b Book) String() string {
	return fmt.Sprintf("Book<Id=%d Title=%q>", b.ID, b.Title)
}

var _ bun.BeforeAppendModelHook = (*Book)(nil)

func (*Book) BeforeAppendModel(ctx context.Context, query bun.Query) error {
	return nil
}

// BookWithCommentCount is like Book model, but has additional CommentCount
// field that is used to select data into it. The use of `bun:",extend"` tag
// is essential here.
type BookWithCommentCount struct {
	Book `bun:",extend"`

	CommentCount int
}

type Translation struct {
	bun.BaseModel `bun:"alias:tr"`

	ID     int    `bun:",pk"`
	BookID int    `bun:"unique:book_id_lang"`
	Book   *Book  `bun:"rel:belongs-to"`
	Lang   string `bun:"unique:book_id_lang"`

	Comments []Comment `bun:"rel:has-many,join:id=trackable_id,join:type=trackable_type,polymorphic"`
}

type Comment struct {
	TrackableID   int    // Book.ID or Translation.ID
	TrackableType string // "book" or "translation"
	Text          string
}

type Department struct {
	bun.BaseModel `bun:"alias:d"`
	CompanyNo     string     `bun:",pk"`
	No            string     `bun:",pk"`
	Employees     []Employee `bun:"rel:has-many,join:company_no=company_no,join:no=department_no"`
}

type Employee struct {
	bun.BaseModel `bun:"alias:p"`
	CompanyNo     string `bun:",pk"`
	DepartmentNo  string `bun:",pk"`
	Name          string `bun:",pk"`
}

func createTestSchema(t *testing.T, db *bun.DB) {
	_ = db.Table(reflect.TypeOf((*BookGenre)(nil)).Elem())

	models := []interface{}{
		(*Image)(nil),
		(*Author)(nil),
		(*Book)(nil),
		(*Genre)(nil),
		(*BookGenre)(nil),
		(*Translation)(nil),
		(*Comment)(nil),
		(*Department)(nil),
		(*Employee)(nil),
	}
	for _, model := range models {
		mustResetModel(t, ctx, db, model)
	}
}

func loadTestData(t *testing.T, ctx context.Context, db *bun.DB) {
	fixture := dbfixture.New(db, dbfixture.WithTruncateTables())
	err := fixture.Load(ctx, os.DirFS("testdata"), "fixture.yaml")
	require.NoError(t, err)
}
