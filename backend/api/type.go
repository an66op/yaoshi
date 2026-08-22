package api

import "github.com/gin-gonic/gin"

// Route 定义单条路由
type Route struct {
	Method      string
	Pattern     string
	Handler     gin.HandlerFunc
	Middlewares []gin.HandlerFunc
}

// RouteGroup 定义路由组
type RouteGroup struct {
	Prefix      string
	Routes      []Route
	Middlewares []gin.HandlerFunc
}
