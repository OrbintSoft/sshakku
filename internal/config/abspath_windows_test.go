//go:build windows

package config

// absRoot is where an absolute path starts on this system. A leading separator
// is not enough here: `\srv\keys` names the root of whichever volume the
// caller happens to be on, so it is resolved against something rather than
// standing on its own. A path is absolute once it names the volume, which is
// why a key_dir written the unix way is joined to the home directory here
// rather than taken as it is.
const absRoot = `C:\`
