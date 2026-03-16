package api

import (
	"ai-hub/server/model"
	"ai-hub/server/store"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
)

// ListMounts 列出所有挂载
func ListMounts(c *gin.Context) {
	mounts, err := store.ListMounts()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if mounts == nil {
		mounts = []model.Mount{}
	}
	c.JSON(http.StatusOK, mounts)
}

// CreateMount 创建挂载
func CreateMount(c *gin.Context) {
	var req struct {
		Alias     string `json:"alias" binding:"required"`
		LocalPath string `json:"local_path" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 验证别名格式（只允许字母数字下划线横线）
	for _, r := range req.Alias {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-') {
			c.JSON(http.StatusBadRequest, gin.H{"error": "alias 只能包含字母、数字、下划线和横线"})
			return
		}
	}

	// 展开路径中的 ~
	localPath := req.LocalPath
	if len(localPath) > 0 && localPath[0] == '~' {
		home, _ := os.UserHomeDir()
		localPath = filepath.Join(home, localPath[1:])
	}

	// 验证本地路径存在且是目录
	info, err := os.Stat(localPath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "本地路径不存在: " + localPath})
		return
	}
	if !info.IsDir() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "本地路径必须是目录"})
		return
	}

	// 检查别名是否已存在
	existing, _ := store.GetMountByAlias(req.Alias)
	if existing != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "别名已存在"})
		return
	}

	m := &model.Mount{
		Alias:     req.Alias,
		LocalPath: localPath,
	}
	if err := store.CreateMount(m); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, m)
}

// DeleteMount 删除挂载
func DeleteMount(c *gin.Context) {
	alias := c.Param("alias")
	if alias == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "alias is required"})
		return
	}

	// 检查是否存在
	existing, _ := store.GetMountByAlias(alias)
	if existing == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "挂载不存在"})
		return
	}

	if err := store.DeleteMountByAlias(alias); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

// ServeStaticMount 提供静态文件服务
func ServeStaticMount(c *gin.Context) {
	alias := c.Param("alias")
	filePath := c.Param("filepath")

	// 获取挂载配置
	m, err := store.GetMountByAlias(alias)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "挂载不存在"})
		return
	}

	// 构建完整路径
	fullPath := filepath.Join(m.LocalPath, filePath)

	// 安全检查：防止路径遍历攻击
	absLocalPath, _ := filepath.Abs(m.LocalPath)
	absFullPath, _ := filepath.Abs(fullPath)
	if len(absFullPath) < len(absLocalPath) || absFullPath[:len(absLocalPath)] != absLocalPath {
		c.JSON(http.StatusForbidden, gin.H{"error": "禁止访问"})
		return
	}

	// 检查文件是否存在
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "文件不存在"})
		return
	}

	c.File(fullPath)
}
