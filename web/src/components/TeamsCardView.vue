<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useChatStore } from '../stores/chat'
import { useRouter } from 'vue-router'
import type { Session } from '../types'
import IconPicker from './IconPicker.vue'

interface Group {
  id: number
  name: string
  icon: string
  description: string
  session_count: number
}

const store = useChatStore()
const router = useRouter()

// Groups data from API
const groups = ref<Group[]>([])
const groupsMap = computed(() => {
  const map: Record<string, Group> = {}
  for (const g of groups.value) {
    map[g.name] = g
  }
  return map
})

// Expanded teams (accordion state)
const expandedTeams = ref<Set<string>>(new Set())

function toggleTeam(teamName: string) {
  const s = new Set(expandedTeams.value)
  if (s.has(teamName)) {
    s.delete(teamName)
  } else {
    s.add(teamName)
  }
  expandedTeams.value = s
}

function isTeamExpanded(teamName: string): boolean {
  return expandedTeams.value.has(teamName)
}

// Group sessions by group_name (exclude sessions without group)
const groupedTeams = computed(() => {
  const result: Record<string, Session[]> = {}
  for (const s of store.sessions) {
    // Skip sessions without group_name
    if (!s.group_name) continue
    if (!result[s.group_name]) result[s.group_name] = []
    result[s.group_name]!.push(s)
  }
  // Sort sessions within each group by updated_at desc
  for (const name in result) {
    result[name]!.sort((a, b) =>
      new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime()
    )
  }
  return result
})

// Count streaming sessions (in conversation)
function getBusyCount(sessions: Session[]): number {
  return sessions.filter(s => s.streaming).length
}

// Get avatar URL for session (use local icons)
function getAvatar(session: Session): string {
  if (session.icon) return `/avatars/${session.icon}`
  const index = (session.id % 50) + 1
  return `/avatars/avatar${index}.svg`
}

// Navigate to session detail
function openSession(sessionId: number) {
  router.push(`/chat/${sessionId}`)
}

// Update team icon
async function updateTeamIcon(teamName: string, icon: string) {
  try {
    await fetch(`/api/v1/groups/${encodeURIComponent(teamName)}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ icon })
    })
    // Refresh groups
    await loadGroups()
  } catch (e) {
    console.error('Failed to update team icon:', e)
  }
}

// Load groups from API
async function loadGroups() {
  try {
    const res = await fetch('/api/v1/groups')
    if (res.ok) {
      groups.value = await res.json()
    }
  } catch (e) {
    console.error('Failed to load groups:', e)
  }
}

// Load sessions on mount
onMounted(() => {
  store.loadSessions()
  loadGroups()
})
</script>

<template>
  <div class="teams-view">
    <div class="teams-header">
      <div>
        <h2 class="teams-title">数字员工</h2>
        <p class="teams-desc">点击团队展开成员列表，点击成员进入对话</p>
      </div>
    </div>

    <div class="teams-grid">
      <div
        v-for="(sessions, teamName) in groupedTeams"
        :key="teamName"
        class="team-card"
        :class="{ expanded: isTeamExpanded(teamName as string) }"
      >
        <!-- Team header (clickable to expand/collapse) -->
        <div class="team-card-header" @click="toggleTeam(teamName as string)">
          <div class="team-icon" @click.stop>
            <IconPicker
              :model-value="groupsMap[teamName as string]?.icon || ''"
              :entity-id="teamName"
              @update:model-value="(icon) => updateTeamIcon(teamName as string, icon)"
            />
          </div>
          <div class="team-header-content">
            <h3 class="team-name">{{ teamName }}</h3>
            <div class="team-badges">
              <span class="team-badge member-badge">{{ sessions.length }} 成员</span>
              <span v-if="getBusyCount(sessions) > 0" class="team-badge active-badge">
                {{ getBusyCount(sessions) }} 对话中
              </span>
            </div>
          </div>
          <svg class="expand-icon" :class="{ rotated: isTeamExpanded(teamName as string) }" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <polyline points="6 9 12 15 18 9"/>
          </svg>
        </div>

        <!-- Members list (accordion content) -->
        <div v-if="isTeamExpanded(teamName as string)" class="team-members">
          <div
            v-for="session in sessions"
            :key="session.id"
            class="member-item"
            @click="openSession(session.id)"
          >
            <img :src="getAvatar(session)" class="member-avatar" :alt="session.title" />
            <div class="member-info">
              <div class="member-name">
                <span v-if="session.streaming || session.process_alive" class="member-status" :class="{ busy: session.streaming }"></span>
                <span class="member-id">#{{ session.id }}</span>
                {{ session.title }}
              </div>
              <div class="member-desc">{{ session.work_dir || '默认工作目录' }}</div>
            </div>
            <svg class="member-arrow" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <polyline points="9 18 15 12 9 6"/>
            </svg>
          </div>
        </div>

        <!-- Collapsed preview -->
        <div v-else class="team-preview" @click="toggleTeam(teamName as string)">
          <div class="team-avatars">
            <div
              v-for="(session, idx) in sessions.slice(0, 4)"
              :key="session.id"
              class="team-avatar"
              :style="{ zIndex: 5 - idx }"
            >
              <img :src="getAvatar(session)" :alt="session.title" />
              <span v-if="session.streaming || session.process_alive" class="avatar-status"></span>
            </div>
            <div v-if="sessions.length > 4" class="team-avatar-more">
              +{{ sessions.length - 4 }}
            </div>
          </div>
          <p class="team-members-preview">
            {{ sessions.slice(0, 3).map(s => s.title).join(', ') }}
            <span v-if="sessions.length > 3">...</span>
          </p>
        </div>
      </div>
    </div>

    <div v-if="Object.keys(groupedTeams).length === 0" class="teams-empty">
      <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
        <path d="M17 21v-2a4 4 0 00-4-4H5a4 4 0 00-4 4v2"/>
        <circle cx="9" cy="7" r="4"/>
        <path d="M23 21v-2a4 4 0 00-3-3.87"/>
        <path d="M16 3.13a4 4 0 010 7.75"/>
      </svg>
      <p>暂无团队，创建会话时选择团队分组</p>
    </div>
  </div>
</template>

<style scoped>
.teams-view {
  padding: 24px;
  overflow-y: auto;
  height: 100%;
}

.teams-header {
  margin-bottom: 24px;
}

.teams-title {
  font-size: 20px;
  font-weight: 600;
  color: var(--text-primary);
  margin: 0 0 4px;
}

.teams-desc {
  font-size: 13px;
  color: var(--text-muted);
  margin: 0;
}

.teams-grid {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.team-card {
  background: var(--bg-secondary);
  border: 1px solid var(--border);
  border-radius: 12px;
  overflow: hidden;
  transition: all 0.2s ease;
}

.team-card:hover {
  border-color: var(--accent);
}

.team-card.expanded {
  border-color: var(--accent);
}

.team-card-header {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 16px 20px;
  cursor: pointer;
  transition: background 0.2s ease;
}

.team-card-header:hover {
  background: var(--bg-hover);
}

.team-icon {
  width: 40px;
  height: 40px;
  border-radius: 10px;
  overflow: hidden;
  flex-shrink: 0;
}

.team-icon :deep(.icon-trigger) {
  border-radius: 10px;
}

.team-icon :deep(.current-icon) {
  width: 40px;
  height: 40px;
}

.team-header-content {
  flex: 1;
  min-width: 0;
}

.team-name {
  font-size: 15px;
  font-weight: 600;
  color: var(--text-primary);
  margin: 0 0 4px;
  transition: color 0.2s ease;
}

.team-card-header:hover .team-name {
  color: var(--accent);
}

.team-badges {
  display: flex;
  gap: 6px;
}

.team-badge {
  font-size: 11px;
  font-weight: 500;
  padding: 2px 8px;
  border-radius: 12px;
}

.member-badge {
  background: var(--bg-hover);
  color: var(--text-muted);
  border: 1px solid var(--border);
}

.active-badge {
  background: rgba(34, 197, 94, 0.1);
  color: #22c55e;
  border: 1px solid rgba(34, 197, 94, 0.2);
}

.expand-icon {
  flex-shrink: 0;
  color: var(--text-muted);
  transition: transform 0.2s ease;
}

.expand-icon.rotated {
  transform: rotate(180deg);
}

/* Members list (accordion content) */
.team-members {
  border-top: 1px solid var(--border);
  background: var(--bg-tertiary);
}

.member-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 20px;
  cursor: pointer;
  transition: background 0.2s ease;
  border-bottom: 1px solid var(--border);
}

.member-item:last-child {
  border-bottom: none;
}

.member-item:hover {
  background: var(--bg-hover);
}

.member-avatar {
  width: 36px;
  height: 36px;
  border-radius: 8px;
  flex-shrink: 0;
}

.member-info {
  flex: 1;
  min-width: 0;
}

.member-name {
  font-size: 14px;
  font-weight: 500;
  color: var(--text-primary);
  display: flex;
  align-items: center;
  gap: 6px;
}

.member-id {
  font-size: 11px;
  color: var(--text-muted);
  background: var(--bg-hover);
  padding: 1px 5px;
  border-radius: 3px;
  font-family: monospace;
}

.member-status {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--warning);
}

.member-status.busy {
  background: var(--success);
}

.member-desc {
  font-size: 12px;
  color: var(--text-muted);
  margin-top: 2px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.member-arrow {
  flex-shrink: 0;
  color: var(--text-muted);
}

/* Collapsed preview */
.team-preview {
  padding: 0 20px 16px;
  cursor: pointer;
}

.team-avatars {
  display: flex;
  align-items: center;
  margin-bottom: 8px;
}

.team-avatar {
  width: 28px;
  height: 28px;
  border-radius: 50%;
  border: 2px solid var(--bg-secondary);
  background: var(--bg-hover);
  overflow: hidden;
  margin-left: -8px;
  position: relative;
}

.team-avatar:first-child {
  margin-left: 0;
}

.team-avatar img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.avatar-status {
  position: absolute;
  bottom: 0;
  right: 0;
  width: 8px;
  height: 8px;
  background: #22c55e;
  border: 2px solid var(--bg-secondary);
  border-radius: 50%;
}

.team-avatar-more {
  width: 28px;
  height: 28px;
  border-radius: 50%;
  border: 2px solid var(--bg-secondary);
  background: var(--bg-hover);
  margin-left: -8px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 10px;
  font-weight: 600;
  color: var(--text-muted);
}

.team-members-preview {
  font-size: 12px;
  color: var(--text-muted);
  margin: 0;
  line-height: 1.4;
  display: -webkit-box;
  -webkit-line-clamp: 1;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.teams-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 60px 20px;
  color: var(--text-muted);
  text-align: center;
}

.teams-empty svg {
  margin-bottom: 16px;
  opacity: 0.5;
}

.teams-empty p {
  font-size: 14px;
  margin: 0;
}

@media (max-width: 768px) {
  .teams-view {
    padding: 16px;
  }

  .team-card-header {
    padding: 12px 16px;
  }

  .member-item {
    padding: 10px 16px;
  }

  .team-preview {
    padding: 0 16px 12px;
  }
}
</style>
