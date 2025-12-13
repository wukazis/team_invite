package middleware

import (
	"net"
	"net/http"

	"github.com/gin-gonic/gin"
)

func AdminIPWhitelist(allowed []string) gin.HandlerFunc {
	if len(allowed) == 0 {
		return func(c *gin.Context) {
			c.Next()
		}
	}
	addrs := make([]net.IP, 0, len(allowed))
	for _, item := range allowed {
		if ip := net.ParseIP(item); ip != nil {
			addrs = append(addrs, ip)
		}
	}
	return func(c *gin.Context) {
		clientIP := net.ParseIP(c.ClientIP())
		for _, allowedIP := range addrs {
			if allowedIP.Equal(clientIP) {
				c.Next()
				return
			}
		}
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "ip not allowed"})
	}
}
