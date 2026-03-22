package sample

import (
	"errors"
	"math"
)

// Add sums two numbers. Supports overflow checking.
func Add(a, b int) (int, error) {
	result := a + b
	if (b > 0 && result < a) || (b < 0 && result > a) {
		return 0, errors.New("integer overflow")
	}
	return result, nil
}

// Subtract subtracts b from a.
func Subtract(a, b int) int {
	return a - b
}

// Divide divides a by b.
func Divide(a, b int) (int, error) {
	if b == 0 {
		return 0, errors.New("division by zero")
	}
	return a / b, nil
}

// Multiply multiplies two numbers.
func Multiply(a, b int) int {
	return a * b
}

// Sqrt returns the square root of a number.
func Sqrt(x float64) (float64, error) {
	if x < 0 {
		return 0, errors.New("negative number")
	}
	return math.Sqrt(x), nil
}

// Modulo returns the remainder of a divided by b.
func Modulo(a, b int) (int, error) {
	if b == 0 {
		return 0, errors.New("division by zero")
	}
	return a % b, nil
}

// Abs returns the absolute value of a number.
func Abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// Power raises base to the power of exp.
// Supports only non-negative exponents.
func Power(base, exp int) (int, error) {
	if exp < 0 {
		return 0, errors.New("negative exponent not supported")
	}
	result := 1
	for i := 0; i < exp; i++ {
		result *= base
	}
	return result, nil
}

// Clamp restricts the value x to the range [min, max].
// Returns an error if min > max.
func Clamp(x, min, max int) (int, error) {
	if min > max {
		return 0, errors.New("min must be <= max")
	}
	if x < min {
		return min, nil
	}
	if x > max {
		return max, nil
	}
	return x, nil
}

// GCD computes the greatest common divisor of two numbers using the Euclidean algorithm.
func GCD(a, b int) int {
	if a < 0 {
		a = -a
	}
	if b < 0 {
		b = -b
	}
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

// LCM computes the least common multiple of two numbers.
func LCM(a, b int) (int, error) {
	if a == 0 || b == 0 {
		return 0, errors.New("LCM undefined for zero")
	}
	g := GCD(a, b)
	return (a / g) * b, nil
}

// Fibonacci returns the n-th Fibonacci number.
// Returns an error if n is negative.
func Fibonacci(n int) (int, error) {
	if n < 0 {
		return 0, errors.New("negative index")
	}
	if n <= 1 {
		return n, nil
	}
	a, b := 0, 1
	for i := 2; i <= n; i++ {
		a, b = b, a+b
	}
	return b, nil
}
