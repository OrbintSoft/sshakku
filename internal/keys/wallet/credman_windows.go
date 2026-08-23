//go:build windows

package wallet

import (
	"errors"
	"fmt"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

// This file is the account's credential store in Go's terms: four calls into
// advapi32 and the spelling a secret is kept in there. Everything above it —
// which entries are sshakku's, what a service name is, when a wallet is asked
// at all — is written once for every platform and is not here.
//
// The store has no command line that will give a secret back (`cmdkey /list`
// names an entry, its type and its user, and never its blob), so the API is not
// one route of several: it is the only one. It is reached as an ordinary export
// of a system DLL, which needs no C compiler and no code generation, so this
// file compiles and vets from any machine even though it can only run on one.
//
// What it stores is a generic credential, kept for this account on this
// computer. Nothing here roams: a passphrase that followed the account onto
// another machine would be a copy nobody asked for.

// The store's own constants, keeping the names they have in Microsoft's
// headers so this code can be checked against the documentation rather than
// against itself.
const (
	credTypeGeneric         = 1
	credPersistLocalMachine = 2

	// maxCredentialBlobSize is CRED_MAX_CREDENTIAL_BLOB_SIZE: the largest
	// secret the store will hold, counted in bytes of the blob rather than in
	// characters of what it spells.
	maxCredentialBlobSize = 5 * 512
)

var (
	advapi32           = windows.NewLazySystemDLL("advapi32.dll")
	procCredReadW      = advapi32.NewProc("CredReadW")
	procCredWriteW     = advapi32.NewProc("CredWriteW")
	procCredDeleteW    = advapi32.NewProc("CredDeleteW")
	procCredEnumerateW = advapi32.NewProc("CredEnumerateW")
	procCredFree       = advapi32.NewProc("CredFree")
)

// credentialW mirrors CREDENTIALW field for field and in order. Its layout is
// what the store reads and writes directly, so a field added, removed or
// reordered here is not a compile error anywhere — it is a call that reads the
// wrong bytes. Attributes is carried as a bare word because this program
// attaches none and always passes nothing.
type credentialW struct {
	Flags              uint32
	Type               uint32
	TargetName         *uint16
	Comment            *uint16
	LastWritten        windows.Filetime
	CredentialBlobSize uint32
	CredentialBlob     *byte
	Persist            uint32
	AttributeCount     uint32
	Attributes         uintptr
	TargetAlias        *uint16
	UserName           *uint16
}

// credential is one entry as the rest of this package sees it: strings, and a
// secret that is a string rather than a blob.
type credential struct {
	// Target is the name the entry is filed under, and is what every other
	// call here takes.
	Target string
	// Comment is the description a person sees beside the entry.
	Comment string
	// User is the account name the entry carries. The store has no use for it
	// on a generic credential, but its own tooling shows it, so an entry with
	// none looks like an entry something failed to finish writing.
	User string
	// Secret is what the entry holds.
	Secret string
}

// blobFromSecret spells a secret the way this system spells a credential blob:
// UTF-16, little end first, with no terminator — the size is carried beside it,
// so a trailing NUL would be part of the secret rather than the end of it.
//
// The spelling matters beyond this program. Any encoding round trips through
// these two functions, since they are each other's inverse; only this one is
// legible to everything else on the machine that reads the store, and a
// passphrase a user cannot read back with their own tooling is one they cannot
// check.
func blobFromSecret(secret string) ([]byte, error) {
	units := utf16.Encode([]rune(secret))
	blob := make([]byte, 2*len(units))
	for i, u := range units {
		blob[2*i] = byte(u)
		blob[2*i+1] = byte(u >> 8)
	}
	if len(blob) > maxCredentialBlobSize {
		return nil, fmt.Errorf("passphrase is too long for the credential store: %d bytes, and it holds %d",
			len(blob), maxCredentialBlobSize)
	}
	return blob, nil
}

// secretFromBlob reads back what blobFromSecret wrote. A trailing odd byte
// cannot come from this program and is dropped rather than guessed at.
func secretFromBlob(blob []byte) string {
	units := make([]uint16, len(blob)/2)
	for i := range units {
		units[i] = uint16(blob[2*i]) | uint16(blob[2*i+1])<<8
	}
	return string(utf16.Decode(units))
}

// credWrite files entry under its target name, replacing whatever was there.
// The store overwrites in place, so nothing above this has to delete first.
func credWrite(entry credential) error {
	target, err := windows.UTF16PtrFromString(entry.Target)
	if err != nil {
		return fmt.Errorf("credential target name: %w", err)
	}
	comment, err := utf16PtrOrNil(entry.Comment)
	if err != nil {
		return fmt.Errorf("credential comment: %w", err)
	}
	user, err := utf16PtrOrNil(entry.User)
	if err != nil {
		return fmt.Errorf("credential user name: %w", err)
	}
	blob, err := blobFromSecret(entry.Secret)
	if err != nil {
		return err
	}

	cred := credentialW{
		Type:               credTypeGeneric,
		TargetName:         target,
		Comment:            comment,
		CredentialBlobSize: uint32(len(blob)),
		Persist:            credPersistLocalMachine,
		UserName:           user,
	}
	if len(blob) > 0 {
		cred.CredentialBlob = &blob[0]
	}

	// The struct is retained for the duration of the call, and everything it
	// points at is reachable from it, so no part of what is handed over can be
	// collected while the store is reading it.
	if ok, err := call(procCredWriteW, uintptr(unsafe.Pointer(&cred)), 0); !ok {
		return fmt.Errorf("write credential %q: %w", entry.Target, err)
	}
	return nil
}

// credRead returns the entry filed under target and whether there was one. An
// entry that is not there is a miss rather than a failure: a key whose
// passphrase was never saved is the ordinary case, not a broken store.
func credRead(target string) (credential, bool, error) {
	name, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return credential{}, false, fmt.Errorf("credential target name: %w", err)
	}

	var cred *credentialW
	ok, err := call(procCredReadW,
		uintptr(unsafe.Pointer(name)), credTypeGeneric, 0, uintptr(unsafe.Pointer(&cred)))
	if !ok {
		if errors.Is(err, windows.ERROR_NOT_FOUND) {
			return credential{}, false, nil
		}
		return credential{}, false, fmt.Errorf("read credential %q: %w", target, err)
	}
	defer free(unsafe.Pointer(cred))

	// Everything is copied out of the store's own memory before it is handed
	// back, since that memory is about to be given up.
	entry := credential{
		Target:  windows.UTF16PtrToString(cred.TargetName),
		Comment: windows.UTF16PtrToString(cred.Comment),
		User:    windows.UTF16PtrToString(cred.UserName),
	}
	if cred.CredentialBlobSize > 0 {
		blob := make([]byte, cred.CredentialBlobSize)
		copy(blob, unsafe.Slice(cred.CredentialBlob, cred.CredentialBlobSize))
		entry.Secret = secretFromBlob(blob)
	}
	return entry, true, nil
}

// credDelete removes the entry filed under target and says whether there was
// one to remove. Both outcomes are success — forgetting an already-forgotten
// key is not a failure — and the caller is still told which happened, because
// a report that says an entry was removed when none was is worse than no report.
func credDelete(target string) (bool, error) {
	name, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return false, fmt.Errorf("credential target name: %w", err)
	}

	ok, err := call(procCredDeleteW, uintptr(unsafe.Pointer(name)), credTypeGeneric, 0)
	if !ok {
		if errors.Is(err, windows.ERROR_NOT_FOUND) {
			return false, nil
		}
		return false, fmt.Errorf("delete credential %q: %w", target, err)
	}
	return true, nil
}

// credList returns the target names of every entry whose name begins with
// prefix. The store answers a prefix query itself — the filter it takes is a
// name followed by an asterisk — so this is the whole of the narrowing, and
// nothing above it filters the answer again.
//
// An empty prefix is refused rather than resolved. It would become the filter
// that matches every credential this account has, from every program on the
// machine, which is the one question this program must never ask.
func credList(prefix string) ([]string, error) {
	if prefix == "" {
		return nil, errors.New("refusing to list credentials under an empty prefix")
	}
	filter, err := windows.UTF16PtrFromString(prefix + "*")
	if err != nil {
		return nil, fmt.Errorf("credential filter: %w", err)
	}

	var count uint32
	var creds **credentialW
	ok, err := call(procCredEnumerateW,
		uintptr(unsafe.Pointer(filter)), 0, uintptr(unsafe.Pointer(&count)), uintptr(unsafe.Pointer(&creds)))
	if !ok {
		// Nothing stored under the prefix is reported the same way as nothing
		// stored at all, and an account that has saved no passphrase yet is
		// the state every account starts in.
		if errors.Is(err, windows.ERROR_NOT_FOUND) {
			return nil, nil
		}
		return nil, fmt.Errorf("list credentials under %q: %w", prefix, err)
	}
	defer free(unsafe.Pointer(creds))

	names := make([]string, 0, count)
	for _, cred := range unsafe.Slice(creds, count) {
		names = append(names, windows.UTF16PtrToString(cred.TargetName))
	}
	return names, nil
}

// call makes one call into the store and turns its answer into Go's. Every one
// of these reports failure by returning false and leaving the reason where the
// last error is kept, so the error is only meaningful once the call has said it
// failed.
func call(proc *windows.LazyProc, args ...uintptr) (bool, error) {
	r1, _, err := proc.Call(args...)
	if r1 == 0 {
		return false, err
	}
	return true, nil
}

// free gives back memory the store allocated for an answer.
func free(p unsafe.Pointer) {
	_, _, _ = procCredFree.Call(uintptr(p))
}

// utf16PtrOrNil converts an optional string, leaving an empty one as the
// nothing the store expects rather than as a pointer to an empty string.
func utf16PtrOrNil(s string) (*uint16, error) {
	if s == "" {
		return nil, nil
	}
	return windows.UTF16PtrFromString(s)
}
