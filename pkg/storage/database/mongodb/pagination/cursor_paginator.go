package pagination
import (
	"context"
	"errors"
	"fmt"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)
type Order string
const (
	OrderAsc  Order = "ASC"
	OrderDesc Order = "DESC"
)
type PaginationRequestDTO struct {
	After  string `json:"after,omitempty"`
	Before string `json:"before,omitempty"`
	Limit  int64  `json:"limit,omitempty"`
}
type CursorPaginationMeta struct {
	Total          *int64 `json:"total,omitempty"`
	StartCursor    string `json:"startCursor,omitempty"`
	EndCursor      string `json:"endCursor,omitempty"`
	HasNextPage    bool   `json:"hasNextPage"`
	HasPreviousPage bool  `json:"hasPreviousPage"`
}
type CursorPaginationResponse struct {
	List []bson.M             `json:"list"`
	Meta CursorPaginationMeta `json:"meta"`
}
type MongoCursorPaginator struct {
	Collection   *mongo.Collection
	CursorColumns []string
	After        string
	Before       string
	Limit        int64
	Order        Order
}
func NewMongoCursorPaginator(
	collection *mongo.Collection,
	cursorColumns []string,
	params PaginationRequestDTO,
	order Order,
) *MongoCursorPaginator {
	limit := params.Limit
	if limit <= 0 {
		limit = 10 // Default limit
	}
	return &MongoCursorPaginator{
		Collection:    collection,
		CursorColumns: cursorColumns,
		After:         params.After,
		Before:        params.Before,
		Limit:         limit,
		Order:         order,
	}
}
func (p *MongoCursorPaginator) Paginate(
	ctx context.Context,
	query bson.M,
	opts *options.FindOptions,
	withTotal bool,
) (*CursorPaginationResponse, error) {
	paginationQuery, err := p.getPaginationQuery()
	if err != nil {
		return nil, err
	}
	finalQuery := query
	if paginationQuery != nil {
		finalQuery = bson.M{"$and": []bson.M{query, paginationQuery}}
	}
	sort := p.buildSort()
	findOpts := options.Find()
	if opts != nil && opts.Projection != nil {
		findOpts.SetProjection(opts.Projection)
	}
	findOpts.SetSort(sort)
	findOpts.SetLimit(p.Limit + 1) // Get one extra to check if there are more results
	cursor, err := p.Collection.Find(ctx, finalQuery, findOpts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var rows []bson.M
	if err := cursor.All(ctx, &rows); err != nil {
		return nil, err
	}
	var total *int64
	if withTotal {
		count, err := p.Collection.CountDocuments(ctx, query)
		if err != nil {
			return nil, err
		}
		total = &count
	}
	hasMore := len(rows) > int(p.Limit)
	if hasMore {
		rows = rows[:len(rows)-1] // Remove the extra item
	}
	if p.After == "" && p.Before != "" {
		for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
			rows[i], rows[j] = rows[j], rows[i]
		}
	}
	hasPreviousPage := p.After != "" || (p.Before != "" && hasMore)
	hasNextPage := p.Before != "" || hasMore
	response := &CursorPaginationResponse{
		List: rows,
		Meta: CursorPaginationMeta{
			Total:          total,
			HasNextPage:    hasNextPage,
			HasPreviousPage: hasPreviousPage,
		},
	}
	if len(rows) > 0 {
		startCursor, err := p.encode(rows[0])
		if err != nil {
			return nil, err
		}
		response.Meta.StartCursor = startCursor
		endCursor, err := p.encode(rows[len(rows)-1])
		if err != nil {
			return nil, err
		}
		response.Meta.EndCursor = endCursor
	}
	return response, nil
}
func (p *MongoCursorPaginator) getPaginationQuery() (bson.M, error) {
	if p.After == "" && p.Before == "" {
		return nil, nil
	}
	var cursors bson.M
	var err error
	if p.After != "" {
		cursors, err = ParseMongoCursor(p.After)
	} else if p.Before != "" {
		cursors, err = ParseMongoCursor(p.Before)
	}
	if err != nil {
		return nil, err
	}
	if !p.isValidCursor(cursors) {
		return nil, errors.New("invalid cursor")
	}
	return p.buildPaginationQuery(cursors), nil
}
func (p *MongoCursorPaginator) buildPaginationQuery(cursors bson.M) bson.M {
	operator := p.getOperator()
	return p.recursivelyBuildQuery(cursors, p.CursorColumns, operator)
}
func (p *MongoCursorPaginator) recursivelyBuildQuery(
	cursors bson.M,
	cursorColumns []string,
	operator string,
) bson.M {
	key := cursorColumns[0]
	if len(cursorColumns) == 1 {
		return bson.M{
			key: bson.M{
				operator: cursors[key],
			},
		}
	} else {
		remainingQuery := p.recursivelyBuildQuery(cursors, cursorColumns[1:], operator)
		return bson.M{
			"$or": []bson.M{
				{
					key: bson.M{
						operator: cursors[key],
					},
				},
				{
					key:       cursors[key],
					"$and": []bson.M{remainingQuery},
				},
			},
		}
	}
}
func (p *MongoCursorPaginator) getOperator() string {
	if p.After != "" {
		if p.Order == OrderAsc {
			return "$gt"
		}
		return "$lt"
	} else if p.Before != "" {
		if p.Order == OrderAsc {
			return "$lt"
		}
		return "$gt"
	}
	return "$gt" // Default fallback
}
func (p *MongoCursorPaginator) buildSort() bson.D {
	order := p.Order
	if p.After == "" && p.Before != "" {
		order = p.reverseOrder(order)
	}
	sortOrder := 1
	if order == OrderDesc {
		sortOrder = -1
	}
	sort := bson.D{}
	for _, column := range p.CursorColumns {
		sort = append(sort, bson.E{Key: column, Value: sortOrder})
	}
	return sort
}
func (p *MongoCursorPaginator) reverseOrder(order Order) Order {
	if order == OrderAsc {
		return OrderDesc
	}
	return OrderAsc
}
func (p *MongoCursorPaginator) encode(row bson.M) (string, error) {
	payload := bson.M{}
	for _, column := range p.CursorColumns {
		if value, ok := row[column]; ok {
			payload[column] = value
		} else {
			return "", fmt.Errorf("cursor column %s not found in result", column)
		}
	}
	return CreateMongoCursor(payload)
}
func (p *MongoCursorPaginator) isValidCursor(cursors bson.M) bool {
	for _, column := range p.CursorColumns {
		if _, ok := cursors[column]; !ok {
			return false
		}
	}
	return true
}