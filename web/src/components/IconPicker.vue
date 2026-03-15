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
        <div class="icon-picker-content" @dragover.prevent="onDragOver" @dragleave="onDragLeave" @drop.prevent="onDrop">
          <div class="picker-header">
            <span>选择图标</span>
            <button class="close-btn" @click="showPicker = false">&times;</button>
          </div>

          <!-- Upload Area -->
          <div
            class="upload-area"
            :class="{ dragging: isDragging, uploading: isUploading }"
            @click="triggerFileInput"
          >
            <input
              ref="fileInput"
              type="file"
              accept=".svg,.png,.jpg,.jpeg,.gif,.webp"
              @change="onFileSelect"
              style="display: none"
            />
            <div v-if="isUploading" class="upload-status">
              <span class="spinner"></span>
              <span>上传中...</span>
            </div>
            <div v-else class="upload-hint">
              <svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/>
                <polyline points="17 8 12 3 7 8"/>
                <line x1="12" y1="3" x2="12" y2="15"/>
              </svg>
              <span>拖拽、点击或粘贴上传图片</span>
              <span class="upload-formats">支持 SVG、PNG、JPG、GIF、WebP</span>
            </div>
          </div>

          <!-- Icons Grid -->
          <div class="icons-section">
            <div v-if="customAvatars.length > 0" class="section-label">已上传</div>
            <div v-if="customAvatars.length > 0" class="icons-grid custom-grid">
              <div
                v-for="icon in customAvatars"
                :key="icon"
                class="icon-item"
                :class="{ selected: icon === modelValue }"
                @click="selectIcon(icon)"
              >
                <img :src="getIconUrl(icon)" :alt="icon" />
              </div>
            </div>

            <div class="section-label">默认图标</div>
            <div class="icons-grid">
              <div
                v-for="icon in defaultAvatars"
                :key="icon"
                class="icon-item"
                :class="{ selected: icon === modelValue }"
                @click="selectIcon(icon)"
              >
                <img :src="getIconUrl(icon)" :alt="icon" />
              </div>
            </div>
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'

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
const fileInput = ref<HTMLInputElement | null>(null)
const isDragging = ref(false)
const isUploading = ref(false)

// Split avatars into custom and default
const customAvatars = computed(() => avatarList.value.filter(a => a.startsWith('custom/')))
const defaultAvatars = computed(() => avatarList.value.filter(a => !a.startsWith('custom/')))

// Generate default icon based on entity ID
const defaultIcon = computed(() => {
  const id = props.entityId || 1
  const index = (Number(id) % 50) + 1
  return `avatar${index}.svg`
})

const currentIconUrl = computed(() => {
  const icon = props.modelValue || defaultIcon.value
  return getIconUrl(icon)
})

const getIconUrl = (icon: string) => {
  if (icon.startsWith('custom/')) {
    return `/avatars/${icon}`
  }
  return `/avatars/${icon}`
}

const selectIcon = (icon: string) => {
  emit('update:modelValue', icon)
  showPicker.value = false
}

const triggerFileInput = () => {
  fileInput.value?.click()
}

const onDragOver = () => {
  isDragging.value = true
}

const onDragLeave = () => {
  isDragging.value = false
}

const onDrop = async (e: DragEvent) => {
  isDragging.value = false
  const files = e.dataTransfer?.files
  if (files && files.length > 0) {
    const file = files[0]
    if (file) await uploadFile(file)
  }
}

const onFileSelect = async (e: Event) => {
  const input = e.target as HTMLInputElement
  if (input.files && input.files.length > 0) {
    const file = input.files[0]
    if (file) await uploadFile(file)
    input.value = '' // Reset for re-upload same file
  }
}

const uploadFile = async (file: File) => {
  // Validate file type
  const allowedTypes = ['image/svg+xml', 'image/png', 'image/jpeg', 'image/gif', 'image/webp']
  if (!allowedTypes.includes(file.type) && !file.name.endsWith('.svg')) {
    alert('不支持的文件格式，请上传 SVG、PNG、JPG、GIF 或 WebP 图片')
    return
  }

  isUploading.value = true
  try {
    const formData = new FormData()
    formData.append('file', file)

    const res = await fetch('/api/v1/avatars/upload', {
      method: 'POST',
      body: formData
    })

    if (res.ok) {
      const data = await res.json()
      // Add to list and select
      avatarList.value.unshift(data.icon)
      emit('update:modelValue', data.icon)
      showPicker.value = false
    } else {
      const err = await res.json()
      alert(err.error || '上传失败')
    }
  } catch (e) {
    alert('上传失败，请重试')
  } finally {
    isUploading.value = false
  }
}

// Handle paste
const onPaste = async (e: ClipboardEvent) => {
  if (!showPicker.value) return

  const items = e.clipboardData?.items
  if (!items) return

  for (const item of items) {
    if (item.type.startsWith('image/')) {
      const file = item.getAsFile()
      if (file) {
        await uploadFile(file)
        break
      }
    }
  }
}

const loadAvatars = async () => {
  try {
    const res = await fetch('/api/v1/avatars')
    if (res.ok) {
      avatarList.value = await res.json()
    }
  } catch (e) {
    // Fallback to generated list
    avatarList.value = Array.from({ length: 50 }, (_, i) => `avatar${i + 1}.svg`)
  }
}

onMounted(() => {
  loadAvatars()
  document.addEventListener('paste', onPaste)
})

onUnmounted(() => {
  document.removeEventListener('paste', onPaste)
})

// Reload avatars when picker opens
watch(showPicker, (val) => {
  if (val) {
    loadAvatars()
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
  max-width: 640px;
  width: 95%;
  max-height: 80vh;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

.picker-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
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

/* Upload Area */
.upload-area {
  border: 2px dashed var(--border-color, #3a3a5c);
  border-radius: 8px;
  padding: 16px;
  text-align: center;
  cursor: pointer;
  transition: all 0.2s;
  margin-bottom: 12px;
}

.upload-area:hover {
  border-color: var(--accent-color, #6366f1);
  background: var(--bg-secondary, #252542);
}

.upload-area.dragging {
  border-color: var(--accent-color, #6366f1);
  background: rgba(99, 102, 241, 0.1);
}

.upload-area.uploading {
  pointer-events: none;
  opacity: 0.7;
}

.upload-hint {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  color: var(--text-secondary, #888);
}

.upload-hint svg {
  opacity: 0.6;
}

.upload-formats {
  font-size: 12px;
  opacity: 0.6;
}

.upload-status {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  color: var(--accent-color, #6366f1);
}

.spinner {
  width: 16px;
  height: 16px;
  border: 2px solid var(--accent-color, #6366f1);
  border-top-color: transparent;
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

/* Icons Section */
.icons-section {
  overflow-y: auto;
  flex: 1;
  min-height: 0;
}

.section-label {
  font-size: 12px;
  color: var(--text-secondary, #888);
  margin: 8px 0 6px;
  padding-left: 4px;
}

.icons-grid {
  display: grid;
  grid-template-columns: repeat(8, 1fr);
  gap: 6px;
  padding: 4px;
}

.icons-grid.custom-grid {
  margin-bottom: 8px;
  padding-bottom: 8px;
  border-bottom: 1px solid var(--border-color, #3a3a5c);
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

/* Mobile */
@media (max-width: 480px) {
  .icons-grid {
    grid-template-columns: repeat(6, 1fr);
  }
}
</style>
