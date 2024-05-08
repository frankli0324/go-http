package h2c

func min(x int, y ...int) int {
	for _, y := range y {
		if y < x {
			x = y
		}
	}
	return x
}
