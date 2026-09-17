# treestamp/watch

Optional fsnotify adapter. It is a separate module so the main Treestamp
library never requires fsnotify.

```text
w, err := watch.Open(root, []string{".gitignore"})
plan, err := w.Plan(ctx)
```

`Plan` maps native create/remove/rename/modify events onto
`treestamp.WatchPlan`. Apply that plan with the library session APIs.
