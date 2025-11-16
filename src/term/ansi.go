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

const Part_foreground = "3"
const Part_background = "4"
const Part_black      = "0"
const Part_red        = "1"
const Part_green      = "2"
const Part_yellow     = "3"
const Part_blue       = "4"
const Part_purple     = "5"
const Part_cyan       = "6"
const Part_white      = "7"

const Hide_cursor = "\x1B[?25l";
const Show_cursor = "\x1B[?25h";
const Move_cursor_fmt = "\x1B[{};{}H";

// https://sw.kovidgoyal.net/kitty/keyboard-protocol/
// This enables an alternative input mode, that makes it possible, among
// others, to unambigously identify the escape key without waiting for
// a timer to run out. And implementing it can be implemented in a backwards
// compatible manner, so that the same code can handle kitty-enabled
// terminals as well as sad terminals.
//
// Must be enabled after entering the alt screen and disabled before leaving
// it.
const Enable_kitty_keyboard = "\x1B[>1u";
const Disable_kitty_keyboard = "\x1B[<u";

const Save_cursor_position = "\x1B[s";
const Save_screen = "\x1B[?47h";
const Enter_alt_buffer = "\x1B[?1049h";

const Leave_alt_buffer = "\x1B[?1049l";
const Restore_screen = "\x1B[?47l";
const Restore_cursor_position = "\x1B[u";

const Clear = "\x1B[2J";

const Reset_attributes = "\x1B[0m";

const Overwrite_mode = "\x1B[4l";

// Per https://espterm.github.io/docs/VT100%20escape%20codes.html
const Enable_auto_wrap = "\x1B[?7h";
const Reset_auto_wrap = "\x1B[?7l";
const Reset_auto_repeat = "\x1B[?8l";
const Reset_auto_interlace = "\x1B[?9l";

const Start_sync = "\x1BP=1s\x1B\\";
const End_sync = "\x1BP=2s\x1B\\";

// 1000: just button tracking.
// 1003: all events, including movement.
//
// This enables standard button tracking. This is limited to maximum XY values
// of 255 - 32 = 223. If anyone complains, we can switch to SGR (1006) encoding,
// however since that is more annoying to parse I decided to take the easy route
// for now. Also this does not provide movement events. Can be enabled with 1003,
// but I don't see that being useful, so it's also not included right not. Let's
// see if anyone complains.
const Enable_mouse_tracking = "\x1B[?1000h";
const Disable_mouse_tracking = "\x1B[?1000l";
