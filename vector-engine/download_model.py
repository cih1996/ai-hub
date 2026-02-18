"""预下载 embedding 模型脚本，在启动 FastAPI 服务之前执行"""

import os
import sys

model_name = os.getenv("EMBEDDING_MODEL_NAME", "BAAI/bge-small-zh-v1.5")
cache_dir = os.getenv("EMBEDDING_MODEL_PATH", os.path.expanduser("~/.ai-hub/vector-engine/models"))

os.makedirs(cache_dir, exist_ok=True)

print(f"[vector] downloading model: {model_name} -> {cache_dir}", flush=True)

from sentence_transformers import SentenceTransformer

model = SentenceTransformer(model_name, device="cpu", cache_folder=cache_dir)

# 验证模型可用
test = model.encode(["test"], show_progress_bar=False)
print(f"[vector] model ready, dim={len(test[0])}", flush=True)
