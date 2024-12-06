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

func (r *Router) Use(middlewares ...Middleware) {
	r.middlewares = append(r.middlewares, middlewares...)
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
