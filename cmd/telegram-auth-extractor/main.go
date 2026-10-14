package main

import "telegram-auth-extractor/internal/config"

func main() {
	_ = config.MustLoad()
}
