<template>
  <div class="icon-picker-wrapper">
    <!-- Trigger: clickable icon -->
    <div class="icon-trigger" @click="showPicker = true" :title="disabled ? '' : '点击更换图标'">
      <img :src="currentIconUrl" alt="icon" class="current-icon" />
      <div v-if="!disabled" class="edit-overlay">
        <svg xmlns="http://www.w3.org/2000/svg" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/>
          <path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"/>
        </svg>
      </div>
    </div>

    <!-- Picker Modal -->
    <Teleport to="body">
      <div v-if="showPicker" class="icon-picker-modal" @click.self="showPicker = false">
        <div class="icon-picker-content">
          <div class="picker-header">
            <span>选择图标</span>
            <button class="close-btn" @click="showPicker = false">&times;</button>
          </div>
          <div class="icons-grid">
            <div
              v-for="icon in avatarList"
              :key="icon"
              class="icon-item"
              :class="{ selected: icon === modelValue }"
              @click="selectIcon(icon)"
            >
              <img :src="`/avatars/${icon}`" :alt="icon" />
            </div>
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'

const props = defineProps<{
  modelValue: string
  entityId?: number | string
  disabled?: boolean
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: string): void
}>()

const showPicker = ref(false)
const avatarList = ref<string[]>([])

// Generate default icon based on entity ID
const defaultIcon = computed(() => {
  const id = props.entityId || 1
  const index = (Number(id) % 50) + 1
  return `avatar${index}.svg`
})

const currentIconUrl = computed(() => {
  const icon = props.modelValue || defaultIcon.value
  return `/avatars/${icon}`
})

const selectIcon = (icon: string) => {
  emit('update:modelValue', icon)
  showPicker.value = false
}

onMounted(async () => {
  try {
    const res = await fetch('/api/v1/avatars')
    if (res.ok) {
      avatarList.value = await res.json()
    }
  } catch (e) {
    // Fallback to generated list
    avatarList.value = Array.from({ length: 50 }, (_, i) => `avatar${i + 1}.svg`)
  }
})
</script>

<style scoped>
.icon-picker-wrapper {
  display: inline-block;
}

.icon-trigger {
  position: relative;
  cursor: pointer;
  border-radius: 8px;
  overflow: hidden;
}

.icon-trigger:hover .edit-overlay {
  opacity: 1;
}

.current-icon {
  width: 40px;
  height: 40px;
  display: block;
}

.edit-overlay {
  position: absolute;
  inset: 0;
  background: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  opacity: 0;
  transition: opacity 0.2s;
  color: white;
}

.icon-picker-modal {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
}

.icon-picker-content {
  background: var(--bg-primary, #1a1a2e);
  border-radius: 12px;
  padding: 16px;
  max-width: 400px;
  width: 90%;
  max-height: 80vh;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

.picker-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
  font-weight: 500;
  color: var(--text-primary, #fff);
}

.close-btn {
  background: none;
  border: none;
  font-size: 24px;
  cursor: pointer;
  color: var(--text-secondary, #888);
  line-height: 1;
}

.close-btn:hover {
  color: var(--text-primary, #fff);
}

.icons-grid {
  display: grid;
  grid-template-columns: repeat(6, 1fr);
  gap: 8px;
  overflow-y: auto;
  max-height: 300px;
  padding: 4px;
}

.icon-item {
  aspect-ratio: 1;
  border-radius: 8px;
  cursor: pointer;
  padding: 4px;
  transition: all 0.2s;
  background: var(--bg-secondary, #252542);
}

.icon-item:hover {
  background: var(--bg-hover, #3a3a5c);
  transform: scale(1.05);
}

.icon-item.selected {
  background: var(--accent-color, #6366f1);
  box-shadow: 0 0 0 2px var(--accent-color, #6366f1);
}

.icon-item img {
  width: 100%;
  height: 100%;
  object-fit: contain;
}
</style>
