"""HTTP agent: real Ollama tool selection, MCP transport, auditable policy gate."""
import asyncio
import json
import os
import time
import uuid
from contextlib import AsyncExitStack
from datetime import datetime, timezone
from pathlib import Path
from typing import Literal

import httpx
from fastapi import FastAPI, HTTPException
from fastapi.responses import Response
from mcp import ClientSession
from mcp.client.streamable_http import streamablehttp_client
from prometheus_client import Counter, Histogram, CONTENT_TYPE_LATEST, generate_latest
from pydantic import BaseModel
from app.gate import REQUIRED_TOOLS, evaluate

app = FastAPI(title="ParcelDesk release readiness agent")
REVIEWS = Counter("release_reviews_total", "Completed review attempts", ["status"])
DURATION = Histogram("release_review_duration_seconds", "Wall clock review duration", buckets=(1,5,10,30,60,120,300,600))
TOOLS = Counter("mcp_tool_calls_total", "MCP tool calls", ["server","tool","status"])
MODELS = Counter("ai_model_calls_total", "Ollama inference calls", ["status"])
TOKENS = Counter("ai_model_tokens_total", "Ollama reported tokens", ["kind"])
for status in ("ready", "blocked", "error"):
    REVIEWS.labels(status).inc(0)
MODEL = os.getenv("MODEL", "qwen3:1.7b")
OUTPUT_DIR = Path(os.getenv("OUTPUT_DIR", "/outputs"))
RULES = (Path(__file__).resolve().parent.parent / "skills/release-review.md").read_text()
SERVER_URLS = {"repo": os.getenv("MCP_REPO_URL", "http://mcp-repo:8001/mcp"),
               "knowledge": os.getenv("MCP_KNOWLEDGE_URL", "http://mcp-knowledge:8002/mcp")}

class ReviewRequest(BaseModel):
    release: Literal["2026.10.1", "2026.10.2"]
    service: Literal["checkout"] = "checkout"

@app.get("/health")
def health():
    return {"status": "ok", "model": MODEL}

@app.get("/metrics")
def metrics():
    return Response(generate_latest(), headers={"Content-Type": CONTENT_TYPE_LATEST})

async def model_call(client, messages, available_tools, trace):
    try:
        body = {"model": MODEL, "messages": messages, "stream": False, "think": False,
                "options": {"temperature": 0, "num_ctx": 8192, "num_predict": 1400}, "keep_alive": "10m"}
        if available_tools:
            body["tools"] = available_tools
        response = await client.post(os.getenv("OLLAMA_URL", "http://ollama:11434") + "/api/chat", json=body)
        response.raise_for_status()
        result = response.json()
        message = result["message"]
        if message.get("role") != "assistant":
            raise ValueError("Unexpected model message role")
        MODELS.labels("success").inc()
        for kind, key in (("input", "prompt_eval_count"), ("output", "eval_count")):
            TOKENS.labels(kind).inc(max(0, result.get(key, 0)))
        trace["model_calls"].append({"message": message, "model": result.get("model"),
            "prompt_tokens": result.get("prompt_eval_count"), "output_tokens": result.get("eval_count")})
        return message
    except Exception:
        MODELS.labels("error").inc()
        raise

async def run_agent(request, trace):
    evidence, inventory, routing = {}, [], {}
    async with AsyncExitStack() as stack:
        for name, url in SERVER_URLS.items():
            read, write, _ = await stack.enter_async_context(streamablehttp_client(url))
            session = await stack.enter_async_context(ClientSession(read, write))
            await session.initialize()
            listed = await session.list_tools()
            trace["discovery"][name] = [tool.name for tool in listed.tools]
            for tool in listed.tools:
                if tool.name not in REQUIRED_TOOLS or tool.name in routing:
                    raise ValueError("Unexpected or duplicate MCP tool")
                routing[tool.name] = (name, session)
                inventory.append({"type":"function", "function":{
                    "name":tool.name, "description":tool.description, "parameters":tool.inputSchema}})
        if set(routing) != REQUIRED_TOOLS:
            raise ValueError("MCP inventory must contain exactly four tools")
        messages = [{"role":"system","content":RULES}, {"role":"user","content":
            f"Review release {request.release}, service {request.service}. First gather all four evidence sources using tools."}]
        async with httpx.AsyncClient(timeout=180) as client:
            for step in range(8):
                remaining = [tool for tool in inventory if tool["function"]["name"] not in evidence]
                message = await model_call(client, messages, remaining, trace)
                messages.append(message)
                calls = message.get("tool_calls") or []
                if not calls:
                    if set(evidence) == REQUIRED_TOOLS:
                        analysis = message.get("content", "").strip()
                        if not analysis:
                            raise ValueError("Model returned empty analysis")
                        return evidence, analysis
                    messages.append({"role":"user", "content":"Evidence is incomplete. Call the remaining tools now: " + ", ".join(sorted(REQUIRED_TOOLS - set(evidence)))})
                    continue
                if len(calls) > 4:
                    raise ValueError("Too many tool calls")
                for call in calls:
                    function = call["function"]
                    name, arguments = function["name"], function["arguments"]
                    if name not in routing or name in evidence:
                        raise ValueError("Unknown or repeated tool")
                    expected = {"service": request.service} if name == "get_release_policy" else {"release":request.release}
                    if arguments != expected:
                        raise ValueError("Tool arguments do not match review request")
                    server, session = routing[name]
                    started = time.monotonic()
                    try:
                        result = await session.call_tool(name, arguments)
                        if result.isError:
                            raise ValueError("MCP tool returned error: " + str(result.content))
                        content = result.structuredContent
                        if content is None:
                            content = json.loads("".join(item.text for item in result.content if item.type == "text"))
                        evidence[name] = content
                        TOOLS.labels(server,name,"success").inc()
                        trace["tool_calls"].append({"server":server,"tool":name,"arguments":arguments,
                            "result":content,"duration_seconds":time.monotonic()-started,"status":"success"})
                    except Exception as exc:
                        TOOLS.labels(server,name,"error").inc()
                        trace["tool_calls"].append({"server":server,"tool":name,"arguments":arguments,"status":"error","error":str(exc)})
                        raise
                    messages.append({"role":"tool","tool_name":name,"content":json.dumps(content,ensure_ascii=False)})
                if set(evidence) == REQUIRED_TOOLS:
                    status, blockers = evaluate(evidence)
                    messages.append({"role":"user","content":f"All evidence collected. Authoritative policy gate: {status}; blockers: {json.dumps(blockers)}. Write the final evidence-grounded analysis now. No more tool calls."})
        raise ValueError("Agent step limit exhausted")

def persist(trace, analysis=None):
    OUTPUT_DIR.mkdir(parents=True, exist_ok=True)
    stem = OUTPUT_DIR / trace["run_id"]
    trace_path = stem.with_suffix(".json")
    temporary = trace_path.with_suffix(".json.tmp")
    temporary.write_text(json.dumps(trace, ensure_ascii=False, indent=2))
    temporary.replace(trace_path)
    if analysis is None:
        return None
    report_path = stem.with_suffix(".md")
    report_path.write_text(f"# ParcelDesk: review {trace['request']['release']}\n\n"
        f"Run: `{trace['run_id']}`\n\nPolicy verdict: **{trace['status']}**\n\n"
        "## Deterministic policy blockers\n\n" + ("\n".join("- " + b for b in trace["blockers"]) or "None.") +
        "\n\n## AI analysis — draft, engineer verification required\n\n" + analysis + "\n\n## Evidence\n\n```json\n" +
        json.dumps(trace["evidence"],ensure_ascii=False,indent=2) + "\n```\n")
    return str(report_path)

@app.post("/review")
async def review(request: ReviewRequest):
    started = time.monotonic()
    trace = {"run_id":uuid.uuid4().hex,"started_at":datetime.now(timezone.utc).isoformat(),
        "request":request.model_dump(),"model":MODEL,"status":"error","discovery":{},"tool_calls":[],"model_calls":[]}
    try:
        async with asyncio.timeout(600):
            evidence, analysis = await run_agent(request, trace)
        status, blockers = evaluate(evidence)
        trace.update(status=status, blockers=blockers, evidence=evidence, analysis=analysis,
                     duration_seconds=time.monotonic()-started)
        report_path = persist(trace, analysis)
        REVIEWS.labels(status).inc()
        return {"run_id":trace["run_id"],"status":status,"blockers":blockers,"report_path":report_path,
                "trace_path":str(OUTPUT_DIR / (trace["run_id"] + ".json")),"analysis":analysis}
    except Exception as exc:
        trace.update(status="error",error=f"{type(exc).__name__}: {exc}",duration_seconds=time.monotonic()-started)
        REVIEWS.labels("error").inc()
        persist(trace)
        raise HTTPException(status_code=502, detail={"run_id":trace["run_id"],"status":"error","error":trace["error"]}) from exc
    finally:
        DURATION.observe(time.monotonic()-started)
