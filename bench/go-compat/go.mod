module github.com/sergii-ziborov/treestamp/bench/go-compat

go 1.23.0

require (
	github.com/boyter/gocodewalker v1.5.1
	github.com/charlievieth/fastwalk v1.0.14
	github.com/karrick/godirwalk v1.17.0
	github.com/sergii-ziborov/treestamp v0.0.0
)

require (
	github.com/danwakefield/fnmatch v0.0.0-20160403171240-cbb64ac3d964 // indirect
	golang.org/x/sync v0.12.0 // indirect
	golang.org/x/sys v0.34.0 // indirect
)

replace github.com/sergii-ziborov/treestamp => ../..
