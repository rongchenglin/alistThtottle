package fs

import (
	"context"

	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/pkg/errors"
)

// ListPage filters only the requested page. Total and HasMore describe the raw
// upstream range, not the filtered output; no refill or whole-directory scan occurs.
func ListPage(ctx context.Context, path string, args model.PageListArgs) (*model.PageList, error) {
	path = utils.FixAndCleanPath(path)
	if len(op.GetStorageVirtualFilesByPath(path)) > 0 {
		return nil, errs.NotSupport
	}
	storage, actualPath, err := op.GetStorageAndActualPath(path)
	if err != nil {
		return nil, errors.WithMessage(err, "failed get storage")
	}
	if _, ok := storage.(driver.PagedReader); !ok {
		return nil, errs.NotImplement
	}
	args.ReqPath = path
	page, err := op.ListPage(ctx, storage, actualPath, args)
	if err != nil {
		return nil, errors.WithMessage(err, "failed get page")
	}
	meta, _ := ctx.Value("meta").(*model.Meta)
	user, _ := ctx.Value("user").(*model.User)
	merge := model.NewObjMerge()
	if whetherHide(user, meta, path) {
		merge.InitHideReg(meta.Hide)
	}
	result := *page
	result.Objs = merge.Merge(page.Objs)
	return &result, nil
}
