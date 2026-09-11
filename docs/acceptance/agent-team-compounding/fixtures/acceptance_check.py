"""Real-runtime sample. Invoke through the assigned Solo Agent, not a PM terminal.

No credentials, database access, network client or service lifecycle operations.
The installed Solo CLI supplies the current Run's normal authorization.
"""
import argparse
import hashlib
import json
import subprocess
import tempfile


def calculate(mode):
    values = [20, 22]
    return {"input": values, "result": sum(values) - (2 if mode == "bad" else 0)}


def cli(*args):
    result = subprocess.run(["solo", *args], capture_output=True, text=True, timeout=30)
    if result.returncode:
        raise RuntimeError("Solo CLI failed; stop and report this error: " + result.stderr)
    return result.stdout


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("mode", choices=["good", "bad", "wait", "resume", "self-check"])
    parser.add_argument("channel", nargs="?")
    parser.add_argument("task", nargs="?")
    args = parser.parse_args()
    if args.mode == "self-check":
        assert calculate("good") == {"input": [20, 22], "result": 42}
        assert calculate("bad") == {"input": [20, 22], "result": 40}
        print("sample arithmetic self-check passed; no Solo flow executed")
        return
    if not args.channel or not args.task:
        parser.error("the Agent must supply the current channel ID and task number")
    task = json.loads(cli("task", "get", "-n", args.task, "-c", args.channel))
    if args.mode == "wait":
        from pathlib import Path
        payload = {
            "expected_task_version": task["version"],
            "idempotency_key": "pm-wait-" + str(task["version"]),
            "condition": {"kind": "signal", "description": "PM 已核对固定输入 20 和 22，允许计算"},
            "handoff": {"summary": "保留原任务，等待 PM 确认输入", "changes": "尚未提交计算结果", "risks": "输入尚未获确认", "next_steps": "收到确认后计算"},
            "next_action": "执行 python3 " + str(Path(__file__).resolve()) + " resume " + args.channel + " " + args.task + " 一次。脚本会提交原任务，完成后停止。",
        }
        command = "wait"
    else:
        if args.mode == "resume":
            waits = json.loads(cli("task", "waits", "-n", args.task, "-c", args.channel))["waits"]
            assert any(item["status"] == "resumed" for item in waits), "original wait must be resumed"
        output = json.dumps(calculate(args.mode), ensure_ascii=False)
        payload = {
            "expected_task_version": task["version"],
            "idempotency_key": "pm-" + args.mode + "-" + str(task["version"]),
            "artifact_version": hashlib.sha256(output.encode()).hexdigest(),
            "handoff": {"summary": "实际计算：" + output, "changes": "执行固定样例脚本并保留输出", "risks": "仅验证给定的两个整数", "next_steps": "请独立核对输入、输出与验收要求"},
            "evidence": [{"id": "E1", "description": "实际 Python 输入与输出", "content": output}],
        }
        command = "submit"
    with tempfile.NamedTemporaryFile(mode="w", encoding="utf-8", suffix=".json") as request:
        json.dump(payload, request, ensure_ascii=False)
        request.flush()
        print(cli("task", command, "-n", args.task, "-c", args.channel, "--file", request.name))
    target = args.channel + ":" + task["message_id"][:8]
    text = "PM_WAIT_REGISTERED：等待 PM 确认输入" if command == "wait" else "PM_ACTUAL_RESULT：" + output
    sent = cli("message", "send", "--target", target, "-c", text)
    print(sent)
    if "HELD:" in sent:
        print("已有新消息；请 Agent 阅读上方补充后处理暂存草稿，不要盲目重发。")


if __name__ == "__main__":
    main()
