// This file is part of zig-spoon, a TUI library for the zig language.
//
// Copyright © 2021 - 2022 Leon Henrik Plickat
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License version 3 as published
// by the Free Software Foundation.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

package term

import (
	"unicode/utf8"
)

//run: go test -v

const (
	TyUnknown int8 = iota
	TyEscape
	TyArrowUp
	TyArrowDown
	TyArrowLeft
	TyArrowRight
	TyBegin
	TyEnd
	TyHome
	TyPageUp
	TyPageDown
	TyDelete
	TyInsert
	TyPrint
	TyScrollLock
	TyPause
	TyFunction
	TyCodepoint
)

const (
	MouseBtn1 int8 = iota
	MouseBtn2
	MouseBtn3
	MouseRelease
	MouseScrollUp
	MouseScrollDown
)

type Event struct {
	Ty int8
	X_len int8
	X rune
	Y uint32
	Button uint

	Mod_alt bool
	Mod_ctrl bool
	Mod_super bool
}
type InputParser []byte;

func (self *InputParser) Next() *Event {
	if 0 == len(*self) {
		return nil
	} else if '\x1B' == (*self)[0] {
		return &Event{ Ty: TyUnknown }
	} else {
		x := self.utf8()
		return &x
	}
	
}

func (self *InputParser) utf8() Event {
	var advance int8 = 1
	defer self.advance(advance)

	switch (*self)[0] {
	case 'a' & '\x1F': return Event{ Ty: TyCodepoint, X: 'a', Mod_ctrl: true }
	case 'b' & '\x1F': return Event{ Ty: TyCodepoint, X: 'b', Mod_ctrl: true }
	case 'c' & '\x1F': return Event{ Ty: TyCodepoint, X: 'c', Mod_ctrl: true }
	case 'd' & '\x1F': return Event{ Ty: TyCodepoint, X: 'd', Mod_ctrl: true }
	case 'e' & '\x1F': return Event{ Ty: TyCodepoint, X: 'e', Mod_ctrl: true }
	case 'f' & '\x1F': return Event{ Ty: TyCodepoint, X: 'f', Mod_ctrl: true }
	case 'g' & '\x1F': return Event{ Ty: TyCodepoint, X: 'g', Mod_ctrl: true }
	case 'h' & '\x1F': return Event{ Ty: TyCodepoint, X: 'h', Mod_ctrl: true }
	case 'i' & '\x1F': return Event{ Ty: TyCodepoint, X: '\t' }
	case 'j' & '\x1F': return Event{ Ty: TyCodepoint, X: '\n' } // Carriage return, which we convert to newline
	case 'k' & '\x1F': return Event{ Ty: TyCodepoint, X: 'k', Mod_ctrl: true }
	case 'l' & '\x1F': return Event{ Ty: TyCodepoint, X: 'l', Mod_ctrl: true }
	case 'm' & '\x1F': return Event{ Ty: TyCodepoint, X: '\n' }
	case 'n' & '\x1F': return Event{ Ty: TyCodepoint, X: 'n', Mod_ctrl: true }
	case 'o' & '\x1F': return Event{ Ty: TyCodepoint, X: 'o', Mod_ctrl: true }
	case 'p' & '\x1F': return Event{ Ty: TyCodepoint, X: 'p', Mod_ctrl: true }
	case 'q' & '\x1F': return Event{ Ty: TyCodepoint, X: 'q', Mod_ctrl: true }
	case 'r' & '\x1F': return Event{ Ty: TyCodepoint, X: 'r', Mod_ctrl: true }
	case 's' & '\x1F': return Event{ Ty: TyCodepoint, X: 's', Mod_ctrl: true }
	case 't' & '\x1F': return Event{ Ty: TyCodepoint, X: 't', Mod_ctrl: true }
	case 'u' & '\x1F': return Event{ Ty: TyCodepoint, X: 'u', Mod_ctrl: true }
	case 'v' & '\x1F': return Event{ Ty: TyCodepoint, X: 'v', Mod_ctrl: true }
	case 'w' & '\x1F': return Event{ Ty: TyCodepoint, X: 'w', Mod_ctrl: true }
	case 'x' & '\x1F': return Event{ Ty: TyCodepoint, X: 'x', Mod_ctrl: true }
	case 'y' & '\x1F': return Event{ Ty: TyCodepoint, X: 'y', Mod_ctrl: true }
	case 'z' & '\x1F': return Event{ Ty: TyCodepoint, X: 'z', Mod_ctrl: true }
	default:
		char, size := utf8.DecodeRune(*self)
		advance = int8(size) // for defer
		if char == utf8.RuneError {
			return Event{ Ty: TyUnknown }
		} else {
			return Event{ Ty: TyCodepoint, X: char }
		}
	}

}

func (self *InputParser) advance(byte_count int8) {
	length := len(*self)
	if int(byte_count) < length {
		*self = (*self)[byte_count:]
	} else {
		*self = (*self)[length:length]
	}
}


