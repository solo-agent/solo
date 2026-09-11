"""Copy into a versioned pm-arithmetic Skill; mode.json is part of the version."""
import hashlib
import json
from pathlib import Path
import subprocess
import sys
import tempfile


def calculate(values, mode, received=None):
    if values != [20, 22]:
        raise ValueError("This fixed case accepts only [20,22]")
    if mode == "receiver":
        if received is None or received["input"] != values or received["result"] != sum(values):
            raise ValueError("Candidate evidence failed independent verification")
        return sum(values)
    if mode not in ("baseline", "candidate"):
        raise ValueError("Unknown version mode")
    return sum(values) - (2 if mode == "baseline" else 0)


def cli(*args):
    done = subprocess.run(["solo", *args], capture_output=True, text=True, timeout=30)
    if done.returncode:
        raise RuntimeError("Stop and report CLI error: " + done.stderr)
    return done.stdout


def main():
    if sys.argv[1:] == ["self-check"]:
        assert calculate([20, 22], "baseline") == 40
        assert calculate([20, 22], "candidate") == 42
        assert calculate([20, 22], "receiver", {"input": [20, 22], "result": 42}) == 42
        try:
            calculate([20, 22], "receiver", {"input": [20, 22], "result": 40})
        except ValueError:
            pass
        else:
            raise AssertionError("receiver accepted incorrect evidence")
        print("selection arithmetic self-check passed; no Solo flow executed")
        return
    channel, number = sys.argv[1:3]
    task = json.loads(cli("task", "get", "-n", number, "-c", channel))
    mode = json.loads(Path(__file__).with_name("mode.json").read_text())["mode"]
    received = json.loads(sys.argv[3]) if mode == "receiver" else None
    if mode == "receiver":
        values = received["input"]
    elif task["title"].startswith("评测："):
        # The product's independent evaluation uses its own fixed description.
        # This exact independent sample is declared in the handbook's requirement.
        values = [20, 22]
    else:
        values = json.loads(task["description"])
    result = calculate(values, mode, received)
    evidence = {"input": values, "result": result, "received_candidate": received, "script": str(Path(__file__).resolve())}
    content = json.dumps(evidence, ensure_ascii=False)
    payload = {
        "expected_task_version": task["version"],
        "idempotency_key": "pm-selection-" + str(task["version"]),
        "artifact_version": hashlib.sha256(content.encode()).hexdigest(),
        "handoff": {"summary": "PM_SELECTION_RESULT_" + str(result), "changes": "实际运行固定版本 Skill", "risks": "只验证一个固定样例", "next_steps": "由 owner 检查真实证据"},
        "evidence": [{"id": "E1", "description": "实际输入、输出与接收的候选证据", "content": content}],
    }
    with tempfile.NamedTemporaryFile(mode="w", encoding="utf-8", suffix=".json") as request:
        json.dump(payload, request, ensure_ascii=False)
        request.flush()
        print(cli("task", "submit", "-n", number, "-c", channel, "--file", request.name))
    print(cli("message", "send", "--target", channel + ":" + task["message_id"][:8], "-c", "PM_SELECTION_RESULT_" + str(result)))


if __name__ == "__main__":
    main()
