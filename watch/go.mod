module github.com/sergii-ziborov/treestamp/watch

go 1.21.0

require (
	github.com/fsnotify/fsnotify v1.9.0
	github.com/sergii-ziborov/treestamp v0.1.0
)

require golang.org/x/sys v0.30.0 // indirect

replace github.com/sergii-ziborov/treestamp => ../
