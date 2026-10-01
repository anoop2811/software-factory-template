package migrateconfigcmd

import "golang.org/x/sys/unix"

func renameNoReplace(directory int, oldName, newName string) error {
	return unix.Renameat2(directory, oldName, directory, newName, unix.RENAME_NOREPLACE)
}
