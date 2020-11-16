package logger

import (
	"fmt"
	"log"
)

type Logger struct {
	Prefix string
}

func (l *Logger) Println(v ...interface{}) {
	a := append([]interface{}{l.Prefix}, v...)
	log.Println(a...)
}
func (l *Logger) Fatalln(v ...interface{}) {
	a := append([]interface{}{l.Prefix}, v...)
	log.Fatalln(a...)
}
func (l *Logger) Printf(format string, v ...interface{}) {
	s := fmt.Sprintf(format, v...)
	log.Print(l.Prefix, s)
}
