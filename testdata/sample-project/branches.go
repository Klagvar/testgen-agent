package sample

func Classify(score int) string {
	if score < 0 {
		return "invalid"
	}
	if score < 50 {
		return "fail"
	}
	if score < 70 {
		return "pass"
	}
	if score < 90 {
		return "good"
	}
	return "excellent"
}

func Abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func Sign(n int) int {
	switch {
	case n > 0:
		return 1
	case n < 0:
		return -1
	default:
		return 0
	}
}

func Max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func Clamp(val, min, max int) int {
	if val < min {
		return min
	}
	if val > max {
		return max
	}
	return val
}

func IsEmpty(s string) bool {
	return len(s) == 0
}
