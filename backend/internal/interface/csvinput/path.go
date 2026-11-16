package csvinput

import "path/filepath"

// ResolvePath は、-csv の値 p を、ファイルを開くパスにする。
// 空は空のまま返す（「CSV_FILE と USER_ID を指定してください」のような引数の不足の案内にするため）。絶対パスはそのまま返す。
// 相対パスは、task を実行したディレクトリ workdir からのパスとして workdir につなぐ
// （task は backend/ で動くので、そのままだと backend/ からのパスに読まれてしまう）。
func ResolvePath(workdir, p string) string {
	// Windows の「\data\t.csv」（ドライブの根からのパス）と「C:t.csv」（ドライブだけが付いた相対パス）は、
	// filepath.IsAbs が false を返すが、workdir につなぐと別の場所になるので、そのまま返す。
	if p == "" || filepath.IsAbs(p) || filepath.VolumeName(p) != "" || p[0] == '\\' || p[0] == '/' {
		return p
	}
	return filepath.Join(workdir, p)
}
