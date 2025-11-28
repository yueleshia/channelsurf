// This is essentially a port of Zig-Spoon
package term

import (
	"fmt"
	"testing"
)

//run: go test

func TestA(t *testing.T) {
	var parser InputParser = []byte("ahello");
	_ = parser
	t.Log(fmt.Sprintf("%c", parser.Next().X))
	t.Log(parser)
}

