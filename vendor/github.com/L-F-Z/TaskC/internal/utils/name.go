package utils

const shortChars = "0123456789abcdefghijklmnopqrstuvwxyz"

func IntToShortName(n int) string {
	if n == 0 {
		return "0"
	}
	result := ""
	for n > 0 {
		remainder := n % 36
		result = string(shortChars[remainder]) + result
		n = n / 36
	}
	return result
}
