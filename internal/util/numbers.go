package util

import (
	"math"
	"strconv"
	"strings"
)

var humanFormatUnits = []string{"", "K", "M", "G", "T", "P"}

func HumanFormat(n int) string {
	if n == 0 {
		return "0"
	}
	if n < 0 {
		return "-" + HumanFormat(-n)
	}
	magnitude := int(math.Floor(math.Log(float64(n)) / math.Log(1000)))
	if magnitude >= len(humanFormatUnits) {
		magnitude = len(humanFormatUnits) - 1
	}
	val := float64(n) / math.Pow(1000, float64(magnitude))
	formatted := strconv.FormatFloat(val, 'f', 1, 64)
	formatted = strings.TrimRight(formatted, "0")
	formatted = strings.TrimRight(formatted, ".")
	return formatted + humanFormatUnits[magnitude]
}
