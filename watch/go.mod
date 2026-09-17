module github.com/sergii-ziborov/treestamp/watch

go 1.23.2

require (
	github.com/fsnotify/fsnotify v1.9.0
	github.com/sergii-ziborov/treestamp v0.1.0
)

require golang.org/x/sys v0.34.0 // indirect

replace github.com/sergii-ziborov/treestamp => ../
