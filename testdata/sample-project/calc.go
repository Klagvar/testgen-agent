package sample

import (
	"errors"
	"math"
)

// Add складывает два числа. Поддерживает проверку на переполнение.
func Add(a, b int) (int, error) {
	result := a + b
	if (b > 0 && result < a) || (b < 0 && result > a) {
		return 0, errors.New("integer overflow")
	}
	return result, nil
}

// Subtract вычитает b из a.
func Subtract(a, b int) int {
	return a - b
}

// Divide делит a на b.
func Divide(a, b int) (int, error) {
	if b == 0 {
		return 0, errors.New("division by zero")
	}
	return a / b, nil
}

// Multiply умножает два числа.
func Multiply(a, b int) int {
	return a * b
}

// Sqrt возвращает квадратный корень числа.
func Sqrt(x float64) (float64, error) {
	if x < 0 {
		return 0, errors.New("negative number")
	}
	return math.Sqrt(x), nil
}

// Modulo возвращает остаток от деления a на b.
func Modulo(a, b int) (int, error) {
	if b == 0 {
		return 0, errors.New("division by zero")
	}
	return a % b, nil
}

// Abs возвращает абсолютное значение числа.
func Abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
