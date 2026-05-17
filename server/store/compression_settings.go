package store

import (
	"ai-hub/server/model"
	"time"
)

const defaultCompressionPrompt = `你是一个专门负责“对话上下文压缩”的 AI 助手。

你的任务是阅读给定的完整历史记录，输出一份高密度、可直接继续工作的压缩总结。

要求：
1. 保留用户目标、约束、偏好、已完成结果、未完成事项、关键决策。
2. 保留重要文件路径、命令、变量名、接口、数据结构、报错结论。
3. 删除寒暄、重复表述、无价值中间过程。
4. 用中文输出。
5. 输出 Markdown，使用以下结构：
   - 用户目标
   - 关键约束
   - 已完成
   - 当前进展
   - 关键资料
   - 下一步
6. 直接输出压缩结果，不要写前言或解释。`

func GetCompressionSettings() (*model.CompressionSettings, error) {
	var s model.CompressionSettings
	var enabled int
	err := DB.QueryRow(
		`SELECT enabled, threshold_percent, system_prompt, updated_at FROM compression_settings WHERE id = 1`,
	).Scan(&enabled, &s.ThresholdPercent, &s.SystemPrompt, &s.UpdatedAt)
	if err != nil {
		return nil, err
	}
	s.Enabled = enabled == 1
	if s.ThresholdPercent <= 0 {
		s.ThresholdPercent = 80
	}
	if s.SystemPrompt == "" {
		s.SystemPrompt = defaultCompressionPrompt
	}
	return &s, nil
}

func UpsertCompressionSettings(s *model.CompressionSettings) error {
	if s == nil {
		return nil
	}
	if s.ThresholdPercent < 0 {
		s.ThresholdPercent = 0
	}
	if s.ThresholdPercent > 100 {
		s.ThresholdPercent = 100
	}
	if s.SystemPrompt == "" {
		s.SystemPrompt = defaultCompressionPrompt
	}
	s.UpdatedAt = time.Now()
	_, err := DB.Exec(
		`INSERT INTO compression_settings (id, enabled, threshold_percent, system_prompt, updated_at)
		 VALUES (1, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   enabled = excluded.enabled,
		   threshold_percent = excluded.threshold_percent,
		   system_prompt = excluded.system_prompt,
		   updated_at = excluded.updated_at`,
		boolToInt(s.Enabled), s.ThresholdPercent, s.SystemPrompt, s.UpdatedAt,
	)
	return err
}
