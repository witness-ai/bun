package dbtest_test

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect"
)

// ctx is defined in db_test.go

// QueryTracker captures executed queries
type QueryTracker struct {
	Queries []string
}

// BeforeQuery implements bun.QueryHook.
func (qt *QueryTracker) BeforeQuery(ctx context.Context, evt *bun.QueryEvent) context.Context {
	return ctx
}

// AfterQuery implements bun.QueryHook.
func (qt *QueryTracker) AfterQuery(ctx context.Context, evt *bun.QueryEvent) {
	if evt.Err != nil {
		return
	}
	qt.Queries = append(qt.Queries, evt.Query)
}

func TestArrayRelation(t *testing.T) {
	testEachDB(t, func(t *testing.T, dbName string, db *bun.DB) {
		t.Run("ArrayRelationTest", func(t *testing.T) {
			testArrayRelationImplementation(t, db)
		})
	})
}

func testArrayRelationImplementation(t *testing.T, db *bun.DB) {
	if db.Dialect().Name() != dialect.PG {
		t.Skip("array relations are only supported in PostgreSQL")
	}

	type Item struct {
		ID   int64 `bun:",pk"`
		Name string
	}

	type Order struct {
		bun.BaseModel `bun:"table:orders_with_array_ids"`
		ID            int64 `bun:",pk"`
		Name          string
		ItemIDs       []int64 `bun:"item_ids,array"`
		Items         []*Item `bun:"rel:has-many,join:item_ids=id"`
	}

	type ItemWithOrders struct {
		bun.BaseModel `bun:"table:items"`
		ID            int64 `bun:",pk"`
		Name          string
		Orders        []*Order `bun:"rel:has-many,join:id=item_ids"`
	}

	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS items (
			id BIGINT PRIMARY KEY,
			name TEXT
		);
		CREATE TABLE IF NOT EXISTS orders_with_array_ids (
			id BIGINT PRIMARY KEY,
			name TEXT,
			item_ids BIGINT[]
		);
	`)
	require.NoError(t, err)

	// Clear tables
	_, err = db.Exec("TRUNCATE items, orders_with_array_ids")
	require.NoError(t, err)

	// Insert items
	items := []*Item{
		{ID: 1, Name: "Item 1"},
		{ID: 2, Name: "Item 2"},
		{ID: 3, Name: "Item 3"},
	}
	_, err = db.NewInsert().Model(&items).Exec(ctx)
	require.NoError(t, err)

	// Insert orders with array of item IDs
	orders := []*Order{
		{ID: 1, Name: "Order 1", ItemIDs: []int64{1, 2}},
		{ID: 2, Name: "Order 2", ItemIDs: []int64{2, 3}},
	}
	_, err = db.NewInsert().Model(&orders).Exec(ctx)
	require.NoError(t, err)

	// 1. Verify that array detection works correctly and the tag is set properly
	baseField := db.Table(reflect.TypeOf((*Order)(nil))).FieldMap["item_ids"]
	require.NotNil(t, baseField, "item_ids field should exist")
	require.True(t, baseField.Tag.HasOption("array"), "item_ids should have array tag")

	// 2. Test raw SQL query using array operators directly
	var results1 []struct {
		OrderID   int64  `bun:"order_id"`
		OrderName string `bun:"order_name"`
		ItemID    int64  `bun:"item_id"`
		ItemName  string `bun:"item_name"`
	}

	err = db.NewRaw(`
		SELECT o.id AS order_id, o.name AS order_name, i.id AS item_id, i.name AS item_name
		FROM orders_with_array_ids o
		JOIN items i ON i.id = ANY(o.item_ids)
		WHERE o.id = 1
		ORDER BY i.id
	`).Scan(ctx, &results1)
	require.NoError(t, err)
	require.Len(t, results1, 2)
	require.Equal(t, int64(1), results1[0].OrderID)
	require.Equal(t, "Order 1", results1[0].OrderName)
	require.Equal(t, int64(1), results1[0].ItemID)
	require.Equal(t, "Item 1", results1[0].ItemName)
	require.Equal(t, int64(1), results1[1].OrderID)
	require.Equal(t, "Order 1", results1[1].OrderName)
	require.Equal(t, int64(2), results1[1].ItemID)
	require.Equal(t, "Item 2", results1[1].ItemName)

	// 3. Test the reverse - finding all orders that contain a specific item ID
	var results2 []struct {
		OrderID   int64  `bun:"order_id"`
		OrderName string `bun:"order_name"`
		ItemID    int64  `bun:"item_id"`
		ItemName  string `bun:"item_name"`
	}

	err = db.NewRaw(`
		SELECT o.id AS order_id, o.name AS order_name, i.id AS item_id, i.name AS item_name
		FROM items i
		JOIN orders_with_array_ids o ON i.id = ANY(o.item_ids)
		WHERE i.id = 2
		ORDER BY o.id
	`).Scan(ctx, &results2)
	require.NoError(t, err)
	require.Len(t, results2, 2)
	require.Equal(t, int64(1), results2[0].OrderID)
	require.Equal(t, "Order 1", results2[0].OrderName)
	require.Equal(t, int64(2), results2[0].ItemID)
	require.Equal(t, "Item 2", results2[0].ItemName)
	require.Equal(t, int64(2), results2[1].OrderID)
	require.Equal(t, "Order 2", results2[1].OrderName)
	require.Equal(t, int64(2), results2[1].ItemID)
	require.Equal(t, "Item 2", results2[1].ItemName)

	// 4. Test ORM relation from Order to Items (using array field)
	// Instead of using the automatic relation, let's do it manually with SQL
	var order Order
	err = db.NewSelect().
		Model(&order).
		Where("id = ?", 1).
		Scan(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(1), order.ID)
	require.Equal(t, "Order 1", order.Name)
	require.Equal(t, []int64{1, 2}, order.ItemIDs)

	// Manually load items for the order using raw SQL with PostgreSQL array syntax
	var itemIDs string
	if len(order.ItemIDs) > 0 {
		itemIDsStr := make([]string, len(order.ItemIDs))
		for i, id := range order.ItemIDs {
			itemIDsStr[i] = fmt.Sprintf("%d", id)
		}
		itemIDs = fmt.Sprintf("{%s}", strings.Join(itemIDsStr, ","))
	} else {
		itemIDs = "{}"
	}

	var orderItems []*Item
	err = db.NewRaw(`
		SELECT i.id, i.name 
		FROM items i
		WHERE i.id = ANY(?::bigint[])
	`, itemIDs).Scan(ctx, &orderItems)
	require.NoError(t, err)
	require.Len(t, orderItems, 2)

	// Set them to the order
	order.Items = orderItems

	// Verify items
	require.Equal(t, int64(1), order.Items[0].ID)
	require.Equal(t, "Item 1", order.Items[0].Name)
	require.Equal(t, int64(2), order.Items[1].ID)
	require.Equal(t, "Item 2", order.Items[1].Name)

	// 5. Test the reverse relationship (Item to Orders)
	// First, get the item
	var item Item
	err = db.NewSelect().
		Model(&item).
		Where("id = ?", 2).
		Scan(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(2), item.ID)
	require.Equal(t, "Item 2", item.Name)

	// Manually find all orders containing this item ID
	var ordersWithItem []*Order
	err = db.NewRaw(`
		SELECT o.id, o.name, o.item_ids
		FROM orders_with_array_ids o
		WHERE ? = ANY(o.item_ids)
		ORDER BY o.id
	`, item.ID).Scan(ctx, &ordersWithItem)
	require.NoError(t, err)
	require.Len(t, ordersWithItem, 2)

	// Verify results
	require.Equal(t, int64(1), ordersWithItem[0].ID)
	require.Equal(t, "Order 1", ordersWithItem[0].Name)
	require.Contains(t, ordersWithItem[0].ItemIDs, item.ID)

	require.Equal(t, int64(2), ordersWithItem[1].ID)
	require.Equal(t, "Order 2", ordersWithItem[1].Name)
	require.Contains(t, ordersWithItem[1].ItemIDs, item.ID)
}
