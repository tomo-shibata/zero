package command

import "errors"

// ErrNotFound は、対象が（ログイン中の利用者に）見つからないこと。API は 404 にする（値段の更新のプラン 4.4）。
var ErrNotFound = errors.New("not found")
