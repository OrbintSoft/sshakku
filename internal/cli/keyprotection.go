package cli

import (
	"github.com/OrbintSoft/sshakku/internal/keys/protect"
)

// keyProtector is what this system can do about encrypting a key file to the
// account that owns it: what the scheme is called, how to ask whether a path is
// covered by it, and how to cover one. The zero value is a system with no such
// scheme, where the report says nothing about protection and the command that
// would turn it on says so and changes nothing.
//
// Only the answers belong to a platform. Everything that reasons over them is
// neutral and takes them as arguments, so both a system that has a scheme and
// one that has none stay exercisable from either machine — which matters here,
// because a machine can only ever give one of the two answers.
type keyProtector struct {
	// scheme names what this system protects a key file with, empty where this
	// build has none here.
	scheme string
	// look answers whether one path is protected, nil where nobody looks.
	look protect.Look
	// apply covers one path, nil where nothing here can.
	apply func(path string) error
}

// keyProtectionHere is what this machine can do about it.
func keyProtectionHere() keyProtector {
	return keyProtectionFor(protect.Scheme())
}

// keyProtectionFor turns the name of a scheme into what the rest of the program
// works with. The name is an argument rather than something read in here, so
// that both answers stay exercisable from either machine: a machine can only
// ever give one of the two, and the arm it does not give is the one nobody
// would otherwise try.
//
// A system with no such scheme is given no way to ask rather than a way that
// always refuses: the report then says nothing about protection at all, which
// is the honest answer where the question does not arise, and no key file is
// opened to establish it.
func keyProtectionFor(scheme string) keyProtector {
	if scheme == "" {
		return keyProtector{}
	}
	return keyProtector{
		scheme: scheme,
		look:   protect.Protected,
		apply:  protect.Protect,
	}
}
