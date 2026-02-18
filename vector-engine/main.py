"""向量引擎 FastAPI 微服务"""

import os
import sys

# 确保能 import 同目录模块
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from fastapi import FastAPI, HTTPException
from pydantic import BaseModel
from vector_db import VectorDB

app = FastAPI(title="AI Hub Vector Engine")

# 两个独立的 collection：knowledge 和 memory
knowledge_db = VectorDB(collection_name="knowledge")
memory_db = VectorDB(collection_name="memory")


def _get_db(scope: str) -> VectorDB:
    if scope == "knowledge":
        return knowledge_db
    elif scope == "memory":
        return memory_db
    raise HTTPException(status_code=400, detail=f"invalid scope: {scope}")


class EmbedRequest(BaseModel):
    scope: str  # "knowledge" | "memory"
    doc_id: str  # 文件路径作为唯一 ID
    text: str  # 文件名 + 内容前200字
    metadata: dict = None


class SearchRequest(BaseModel):
    scope: str
    query: str
    top_k: int = 5


class DeleteRequest(BaseModel):
    scope: str
    doc_id: str


@app.post("/embed")
def embed(req: EmbedRequest):
    db = _get_db(req.scope)
    db.add(doc_id=req.doc_id, text=req.text, metadata=req.metadata)
    return {"status": "ok", "doc_id": req.doc_id}


@app.post("/search")
def search(req: SearchRequest):
    db = _get_db(req.scope)
    results = db.search(query=req.query, top_k=req.top_k)
    # 记录命中
    for item in results:
        db.record_hit(item["id"])
    return {"results": results}


@app.post("/delete")
def delete(req: DeleteRequest):
    db = _get_db(req.scope)
    db.delete(doc_id=req.doc_id)
    return {"status": "ok", "doc_id": req.doc_id}


@app.get("/health")
def health():
    return {
        "status": "ok",
        "knowledge_count": knowledge_db.collection.count(),
        "memory_count": memory_db.collection.count(),
    }


@app.get("/stats")
def stats(scope: str = "knowledge"):
    db = _get_db(scope)
    return db.stats()


if __name__ == "__main__":
    import uvicorn

    port = int(os.getenv("VECTOR_ENGINE_PORT", "8090"))
    uvicorn.run(app, host="127.0.0.1", port=port, log_level="info")
