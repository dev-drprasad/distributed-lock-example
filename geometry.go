package main

// import (
// 	"log"
// 	"math"
// )

// func getDirection(a position, b position, c position) float64 {
// 	theta1 := GetAngle(a, b)
// 	theta2 := GetAngle(b, c)
// 	angle := (theta1 - theta2)
// 	log.Println("angle", angle)
// 	// if angle < 0 {
// 	// 	return angle + 2*math.Pi
// 	// }
// 	return math.Mod(angle, math.Pi)

// }

// func GetAngle(p1 position, p2 position) float64 {
// 	if (p2.X - p1.X) == 0 {
// 		return 0
// 	}
// 	angleFromXAxis := math.Atan(float64((p2.Y - p1.Y) / (p2.X - p1.X)))
// 	if p2.X-p1.X < 0 {
// 		return angleFromXAxis + math.Pi
// 	} else {
// 		return angleFromXAxis
// 	}
// }
