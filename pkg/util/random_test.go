package util

import "testing"

func Test_generateRandomStr(t *testing.T) {
	str := GenerateRandomStr(5)
	t.Log(str)
}
