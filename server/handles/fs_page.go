package handles

import (
	"context"
	"errors"
	"math"
	stdpath "path"

	"github.com/alist-org/alist/v3/internal/fs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/server/common"
	"github.com/gin-gonic/gin"
)

func sourcePageArgs(req ListReq) (model.PageListArgs, error) {
	if req.Page < 1 || req.PerPage < 1 || req.PerPage > MaxPerPage || int64(req.Page) > math.MaxInt64/int64(req.PerPage) {
		return model.PageListArgs{}, errors.New("invalid page range")
	}
	switch req.OrderBy {
	case "", "name", "size", "modified":
	default:
		return model.PageListArgs{}, errors.New("unsupported order_by")
	}
	switch req.OrderDirection {
	case "", "asc", "desc":
	default:
		return model.PageListArgs{}, errors.New("unsupported order_direction")
	}
	return model.PageListArgs{
		ListArgs: model.ListArgs{Refresh: req.Refresh, NoUpdateIndex: true},
		Offset:   int64(req.Page-1) * int64(req.PerPage), Limit: int64(req.PerPage),
		OrderBy: req.OrderBy, OrderDirection: req.OrderDirection,
	}, nil
}

func respondSourcePage(c *gin.Context, reqPath string, req ListReq, user *model.User, meta *model.Meta, perm int32, provider string) {
	args, err := sourcePageArgs(req)
	if err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	ctx := context.WithValue(c.Request.Context(), "user", user)
	ctx = context.WithValue(ctx, "meta", meta)
	page, err := fs.ListPage(ctx, reqPath, args)
	if err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	visible := make([]model.Obj, 0, len(page.Objs))
	for _, obj := range page.Objs {
		if common.CanReadPathByRole(user, stdpath.Join(reqPath, obj.GetName())) {
			visible = append(visible, obj)
		}
	}
	// Totals are upstream, not an exact count after local permissions/hide rules.
	// Do not crawl other ranges merely to compute totals or fill the page.
	content := toObjsResp(visible, reqPath, isEncrypt(meta, reqPath))
	if content == nil {
		content = []ObjLabelResp{}
	}
	common.SuccessResp(c, FsListResp{
		SourcePagination: true, TotalExact: false,
		Content: content, Total: page.Total, FilteredTotal: page.Total,
		Page: req.Page, PerPage: req.PerPage, HasMore: page.HasMore,
		PagesTotal: 0,
		Readme:     getReadme(meta, reqPath), Header: getHeader(meta, reqPath),
		Write:    common.HasPermission(perm, common.PermWrite) || common.CanWrite(meta, reqPath),
		Provider: provider,
	})
}
