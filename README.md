# godrive-fuse
FUSE Google Drive client in go

## CURRENT STATE: BARELY USABLE

Currently it's still in early age and rapid development.
It might cause you to lose data.
THERE ARE DRAGONS.
You have been warned. Use at your own risk.

Currently supported features:

- ls
- read files
- rmdir
- mkdir
- create new files

There are some weird permission warnings when trying to rm or rmdir
(but it still works),
and overwriting an existing file fails for likely similar reasons.
