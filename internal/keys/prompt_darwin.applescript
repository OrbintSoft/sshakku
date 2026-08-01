-- Asks for one key's passphrase and writes what was typed to standard output.
-- A dismissed dialog exits non-zero, which is how the caller tells "cancelled"
-- from "answered with nothing".
--
-- The key's name arrives as an argument rather than being written into this
-- script. A name pasted into the source could close the string it landed in
-- and continue as AppleScript of its own, and key files are named by whoever
-- put them in ~/.ssh.
on run argv
	set keyName to item 1 of argv
	set reply to display dialog "Enter passphrase for " & keyName default answer "" with hidden answer with title "SSHakku"
	return text returned of reply
end run
