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

// Group sessions by group_name
const groupedTeams = computed(() => {
  const result: Record<string, Session[]> = {}
  for (const s of store.sessions) {
    const name = s.group_name || '未分组'
    if (!result[name]) result[name] = []
    result[name].push(s)
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

// Navigate to team's first session
function openTeam(teamName: string) {
  const sessions = groupedTeams.value[teamName]
  if (sessions && sessions.length > 0) {
    router.push(`/chat/${sessions[0]!.id}`)
  }
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
        <h2 class="teams-title">团队列表</h2>
        <p class="teams-desc">选择一个团队进行管理和对话</p>
      </div>
    </div>

    <div class="teams-grid">
      <div
        v-for="(sessions, teamName) in groupedTeams"
        :key="teamName"
        class="team-card"
        @click="openTeam(teamName as string)"
      >
        <div class="team-card-header">
          <div class="team-icon" @click.stop>
            <IconPicker
              :model-value="groupsMap[teamName as string]?.icon || ''"
              :entity-id="teamName"
              @update:model-value="(icon) => updateTeamIcon(teamName as string, icon)"
            />
          </div>
          <div class="team-badges">
            <span class="team-badge member-badge">{{ sessions.length }} 成员</span>
            <span v-if="getBusyCount(sessions) > 0" class="team-badge active-badge">
              {{ getBusyCount(sessions) }} 对话中
            </span>
          </div>
        </div>

        <h3 class="team-name">{{ teamName }}</h3>
        <p class="team-members-preview">
          {{ sessions.slice(0, 3).map(s => s.title).join(', ') }}
          <span v-if="sessions.length > 3">...</span>
        </p>

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
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 16px;
}

.team-card {
  background: var(--bg-secondary);
  border: 1px solid var(--border);
  border-radius: 12px;
  padding: 20px;
  cursor: pointer;
  transition: all 0.2s ease;
}

.team-card:hover {
  border-color: var(--accent);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
  transform: translateY(-2px);
}

.team-card-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: 12px;
}

.team-icon {
  width: 40px;
  height: 40px;
  border-radius: 10px;
  overflow: hidden;
}

.team-icon :deep(.icon-trigger) {
  border-radius: 10px;
}

.team-icon :deep(.current-icon) {
  width: 40px;
  height: 40px;
}

.team-badges {
  display: flex;
  gap: 6px;
}

.team-badge {
  font-size: 11px;
  font-weight: 500;
  padding: 3px 8px;
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

.team-name {
  font-size: 16px;
  font-weight: 600;
  color: var(--text-primary);
  margin: 0 0 6px;
  transition: color 0.2s ease;
}

.team-card:hover .team-name {
  color: var(--accent);
}

.team-members-preview {
  font-size: 12px;
  color: var(--text-muted);
  margin: 0 0 16px;
  line-height: 1.4;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
  min-height: 34px;
}

.team-avatars {
  display: flex;
  align-items: center;
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

  .teams-grid {
    grid-template-columns: 1fr;
  }
}
</style>
