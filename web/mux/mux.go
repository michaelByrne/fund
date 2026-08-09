package mux

import "net/http"

type Middleware func(handlerFunc http.HandlerFunc) http.HandlerFunc

type Mux interface {
	Handle(pattern string, handler http.Handler)
	HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request))
	ServeHTTP(http.ResponseWriter, *http.Request)
}

type Router struct {
	mux         Mux
	middlewares []Middleware
}

func NewRouter(mux Mux, middlewares ...Middleware) *Router {
	return &Router{
		mux:         mux,
		middlewares: middlewares,
	}
}

// Use adds middleware, applied to every route registered after it, outermost
// first.
//
// It took ...interface{} and decided at runtime, with a panic for anything it
// did not recognise. That panic reached production, and the reason is a rule
// that is easy to forget: a type switch matches the *defined* type, so
// `case Middleware` did not match a plain func(http.HandlerFunc)
// http.HandlerFunc. The two are assignable and not identical, so the value went
// straight past the case that was written for it and into the default.
//
// A parameter of type Middleware accepts that same function without complaint,
// because assignability is what governs arguments. Nothing else changes, except
// that the mistake is now a compile error in a place someone is looking at
// rather than a panic on the first boot after deploy.
func (r *Router) Use(middlewares ...Middleware) {
	r.middlewares = append(r.middlewares, middlewares...)
}

// UseHandler adds middleware written against http.Handler, which is what most
// third-party middleware is -- scs's LoadAndSave among them.
//
// A separate method rather than another case in a switch. There is no way to
// overload on parameter type in Go, and the alternative was the interface{}
// above.
func (r *Router) UseHandler(middlewares ...func(http.Handler) http.Handler) {
	for _, middleware := range middlewares {
		r.middlewares = append(r.middlewares, adaptHandlerMiddleware(middleware))
	}
}

func (r *Router) Handle(pattern string, handler http.Handler) {
	r.mux.Handle(pattern, compileHandlerWithMiddleware(r.middlewares, handler.ServeHTTP))
}

func (r *Router) HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	r.mux.HandleFunc(pattern, compileHandlerWithMiddleware(r.middlewares, handler))
}

func (r *Router) ListenAndServe(addr string, handler http.Handler) error {
	return http.ListenAndServe(addr, handler)
}

func (r *Router) ListenAndServeTLS(addr, certFile, keyFile string, handler http.Handler) error {
	return http.ListenAndServeTLS(addr, certFile, keyFile, handler)
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mux.ServeHTTP(w, req)
}

func compileHandlerWithMiddleware(middlewares []Middleware, f http.HandlerFunc) http.HandlerFunc {
	for i := len(middlewares) - 1; i >= 0; i-- {
		f = middlewares[i](f)
	}

	return f
}

func adaptHandlerMiddleware(handlerMiddleware func(http.Handler) http.Handler) Middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			handlerMiddleware(next).ServeHTTP(w, r)
		}
	}
}
