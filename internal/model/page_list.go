package model

// PageListArgs describes a bounded upstream range, not a complete directory.
type PageListArgs struct {
	ListArgs
	Offset         int64
	Limit          int64
	OrderBy        string
	OrderDirection string
}

// PageList contains one upstream range. Total is before local visibility filters.
type PageList struct {
	Objs    []Obj
	Total   int64
	HasMore bool
}
