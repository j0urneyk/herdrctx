package herdr

import "testing"

func TestInsideHerdr(t *testing.T) {
	t.Parallel()

	if !InsideHerdr([]string{"HERDR_ENV=1"}) {
		t.Fatal("InsideHerdr() = false, want true")
	}
	if !InsideHerdr([]string{"HERDR_SOCKET_PATH=/tmp/herdr.sock"}) {
		t.Fatal("InsideHerdr() = false for HERDR_SOCKET_PATH, want true")
	}
	if !InsideHerdr([]string{"HERDR_ENV=0", "HERDR_SOCKET_PATH=/tmp/herdr.sock"}) {
		t.Fatal("InsideHerdr() = false for socket-only context, want true")
	}
	if InsideHerdr([]string{"HERDR_ENV=0"}) {
		t.Fatal("InsideHerdr() = true, want false")
	}
	if InsideHerdr([]string{"HERDR_SOCKET_PATH="}) {
		t.Fatal("InsideHerdr() = true for empty HERDR_SOCKET_PATH, want false")
	}
	if InsideHerdr([]string{"OTHER=1"}) {
		t.Fatal("InsideHerdr() = true, want false")
	}
}
