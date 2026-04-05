package quicframe

import (
	"fmt"
	"strings"
)

// HandlerFunc is the core handler signature.
type HandlerFunc func(ctx *Context) error

// MiddlewareFunc wraps a HandlerFunc, enabling pre- and post-processing.
// Middleware MUST call next(ctx) to continue the chain.
type MiddlewareFunc func(next HandlerFunc) HandlerFunc

// route holds a compiled route pattern and its handler chain.
type route struct {
	method   string
	segments []segment // parsed path segments
	handler  HandlerFunc
}

type segmentKind uint8

const (
	segStatic   segmentKind = iota // literal match: "users"
	segParam                       // named parameter: ":id"
	segWildcard                    // tail wildcard: "*rest"
)

type segment struct {
	kind  segmentKind
	value string // literal text, param name, or wildcard name
}

// Router performs method + path matching and dispatches to handlers.
type Router struct {
	routes     []*route
	middleware []MiddlewareFunc

	// NotFound is called when no route matches the path.
	NotFound HandlerFunc
	// MethodNotAllowed is called when the path matches but not the method.
	MethodNotAllowed HandlerFunc
}

func newRouter() *Router {
	r := &Router{}
	r.NotFound = func(ctx *Context) error {
		return ctx.Error(404, fmt.Sprintf("no route for %s %s", ctx.Method(), ctx.Path()))
	}
	r.MethodNotAllowed = func(ctx *Context) error {
		return ctx.Error(405, fmt.Sprintf("method %s not allowed on %s", ctx.Method(), ctx.Path()))
	}
	return r
}

// add registers a handler for the given method and pattern.
func (r *Router) add(method, pattern string, middleware []MiddlewareFunc, handler HandlerFunc) {
	segs := parsePattern(pattern)
	h := applyMiddleware(handler, middleware)
	r.routes = append(r.routes, &route{
		method:   strings.ToUpper(method),
		segments: segs,
		handler:  h,
	})
}

// match finds the best matching route for the given method and path.
// Returns the handler (with middleware applied) and any path params, or an error handler.
func (r *Router) match(method, path string) (HandlerFunc, map[string]string) {
	pathSegs := splitPath(path)

	var pathMatched bool
	for _, rt := range r.routes {
		params, ok := matchSegments(rt.segments, pathSegs)
		if !ok {
			continue
		}
		pathMatched = true
		if rt.method != strings.ToUpper(method) {
			continue
		}
		return rt.handler, params
	}

	if pathMatched {
		return r.MethodNotAllowed, nil
	}
	return r.NotFound, nil
}

// ─── Pattern parsing ─────────────────────────────────────────────────────────

func parsePattern(pattern string) []segment {
	parts := splitPath(pattern)
	segs := make([]segment, 0, len(parts))
	for _, p := range parts {
		switch {
		case strings.HasPrefix(p, ":"):
			segs = append(segs, segment{kind: segParam, value: p[1:]})
		case strings.HasPrefix(p, "*"):
			name := p[1:]
			if name == "" {
				name = "wildcard"
			}
			segs = append(segs, segment{kind: segWildcard, value: name})
		default:
			segs = append(segs, segment{kind: segStatic, value: p})
		}
	}
	return segs
}

func splitPath(path string) []string {
	path = strings.Trim(path, "/")
	if path == "" {
		return []string{}
	}
	return strings.Split(path, "/")
}

// matchSegments returns path params if routeSegs matches actualSegs.
func matchSegments(routeSegs []segment, actualSegs []string) (map[string]string, bool) {
	params := make(map[string]string)

	ri, ai := 0, 0
	for ri < len(routeSegs) && ai < len(actualSegs) {
		rs := routeSegs[ri]
		switch rs.kind {
		case segStatic:
			if rs.value != actualSegs[ai] {
				return nil, false
			}
		case segParam:
			params[rs.value] = actualSegs[ai]
		case segWildcard:
			// consume the rest of actualSegs
			params[rs.value] = strings.Join(actualSegs[ai:], "/")
			return params, true
		}
		ri++
		ai++
	}

	// Both must be exhausted for a full match (wildcards return early above).
	if ri == len(routeSegs) && ai == len(actualSegs) {
		return params, true
	}
	return nil, false
}

// ─── Middleware chaining ─────────────────────────────────────────────────────

// applyMiddleware wraps handler with middleware in declaration order.
// The first middleware in the slice is the outermost wrapper.
func applyMiddleware(handler HandlerFunc, mw []MiddlewareFunc) HandlerFunc {
	for i := len(mw) - 1; i >= 0; i-- {
		handler = mw[i](handler)
	}
	return handler
}

// ─── Route group ─────────────────────────────────────────────────────────────

// Group provides scoped middleware and prefix for a set of routes.
type Group struct {
	app        *App
	prefix     string
	middleware []MiddlewareFunc
}

// Use appends middleware scoped to this group.
func (g *Group) Use(mw ...MiddlewareFunc) *Group {
	g.middleware = append(g.middleware, mw...)
	return g
}

func (g *Group) handle(method, path string, handler HandlerFunc) {
	g.app.addRoute(method, g.prefix+path, g.middleware, handler)
}

// GET registers a GET handler.
func (g *Group) GET(path string, handler HandlerFunc) *Group {
	g.handle("GET", path, handler)
	return g
}

// POST registers a POST handler.
func (g *Group) POST(path string, handler HandlerFunc) *Group {
	g.handle("POST", path, handler)
	return g
}

// PUT registers a PUT handler.
func (g *Group) PUT(path string, handler HandlerFunc) *Group {
	g.handle("PUT", path, handler)
	return g
}

// DELETE registers a DELETE handler.
func (g *Group) DELETE(path string, handler HandlerFunc) *Group {
	g.handle("DELETE", path, handler)
	return g
}

// PATCH registers a PATCH handler.
func (g *Group) PATCH(path string, handler HandlerFunc) *Group {
	g.handle("PATCH", path, handler)
	return g
}
