package filepublish

import "golang.org/x/sys/unix"

func renameNoReplace(directory int, oldName, newName string) error {
	return unix.RenameatxNp(directory, oldName, directory, newName, unix.RENAME_EXCL)
}
