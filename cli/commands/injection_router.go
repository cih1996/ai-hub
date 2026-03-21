package commands

import (
	"ai-hub/cli/client"
	"encoding/json"
	"fmt"
	"os"
)

// RunInjectionRouter executes the injection-router command
func RunInjectionRouter(c *client.Client, args []string) int {
	if len(args) == 0 {
		// Default to list
		return injectionRouterList(c)
	}

	switch args[0] {
	case "list":
		return injectionRouterList(c)
	case "set":
		return injectionRouterSet(c, args[1:])
	case "delete":
		return injectionRouterDelete(c, args[1:])
	case "--help":
		printInjectionRouterHelp()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "Unknown injection-router subcommand: %s\n", args[0])
		printInjectionRouterHelp()
		return 1
	}
}

func injectionRouterList(c *client.Client) int {
	respData, err := c.GET("/injection-router")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	var resp struct {
		Routes []struct {
			ID               int64  `json:"id"`
			Keywords         string `json:"keywords"`
			InjectCategories string `json:"inject_categories"`
			CreatedAt        string `json:"created_at"`
			UpdatedAt        string `json:"updated_at"`
		} `json:"routes"`
		Categories  []string `json:"categories"`
		Fixed       []string `json:"fixed"`
		Conditional []string `json:"conditional"`
	}
	if err := json.Unmarshal(respData, &resp); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		return 1
	}

	// Print category info
	fmt.Println("=== 记忆分类 ===")
	fmt.Printf("固定注入（每次必带）: %v\n", resp.Fixed)
	fmt.Printf("按需注入（关键词匹配）: %v\n", resp.Conditional)
	fmt.Println()

	// Print routes
	if len(resp.Routes) == 0 {
		fmt.Println("暂无路由规则。使用 'ai-hub injection-router set' 添加。")
		return 0
	}

	fmt.Printf("=== 路由规则（%d 条）===\n\n", len(resp.Routes))
	for _, r := range resp.Routes {
		fmt.Printf("#%-4d  关键词: %s\n", r.ID, r.Keywords)
		fmt.Printf("       注入: %s\n", r.InjectCategories)
		fmt.Printf("       更新: %s\n", FormatTime(r.UpdatedAt))
		fmt.Println("---")
	}
	return 0
}

func injectionRouterSet(c *client.Client, args []string) int {
	var keywords, inject string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--keywords":
			if i+1 < len(args) {
				i++
				keywords = args[i]
			}
		case "--inject":
			if i+1 < len(args) {
				i++
				inject = args[i]
			}
		}
	}

	if keywords == "" || inject == "" {
		fmt.Fprintf(os.Stderr, "Usage: ai-hub injection-router set --keywords \"开发|编程|代码\" --inject \"domain,lessons\"\n")
		return 1
	}

	body := map[string]interface{}{
		"keywords":          keywords,
		"inject_categories": inject,
	}

	respData, err := c.POST("/injection-router", body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	var resp struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(respData, &resp); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		return 1
	}

	fmt.Printf("路由规则 #%d 已创建。\n", resp.ID)
	fmt.Printf("  关键词: %s\n", keywords)
	fmt.Printf("  注入分类: %s\n", inject)
	return 0
}

func injectionRouterDelete(c *client.Client, args []string) int {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Usage: ai-hub injection-router delete <id>\n")
		return 1
	}

	routeID := args[0]
	_, err := c.DELETE(fmt.Sprintf("/injection-router/%s", routeID))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	fmt.Printf("路由规则 #%s 已删除。\n", routeID)
	return 0
}

func printInjectionRouterHelp() {
	fmt.Fprintf(os.Stderr, `Usage: ai-hub injection-router <subcommand> [args]

管理记忆注入路由规则。新会话第一条消息到达时，根据关键词匹配决定注入哪些记忆分类。

分类说明：
  identity       用户身份画像（固定注入）
  preferences    用户偏好习惯（固定注入）
  error-genome   AI 常犯错误模式库（固定注入）
  domain         用户领域知识（按需注入）
  lessons        踩过的坑和教训（按需注入）
  active         当前进行中的事项（按需注入）
  decisions      重要决策记录（按需注入）

Subcommands:
  list                                           查看所有路由规则
  set --keywords "开发|编程" --inject "domain,lessons"  创建路由规则
  delete <id>                                    删除路由规则

Examples:
  ai-hub injection-router list
  ai-hub injection-router set --keywords "开发|编程|代码|网站" --inject "domain,lessons,decisions"
  ai-hub injection-router set --keywords "购物|推荐|买" --inject "lessons,preferences"
  ai-hub injection-router delete 1
`)
}
