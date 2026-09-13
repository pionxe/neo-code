#!/usr/bin/env python3
# S5 真实网关冒烟（issue #46 C7）：
# 1. 构建并拉起真实 neocode-gateway（--http-listen 127.0.0.1:0，临时 HOME）
# 2. tuiv2 --backend=gateway --gateway-address=<解析地址> pty 启动
# 3. 断言：初始帧渲染 / 创建会话入口可用 / 退出码 0
# 范围说明：LLM 全链路（流式/工具/问答）依赖真实 provider key，归 S10 全量联调。
import os, pty, re, select, signal, subprocess, sys, tempfile, time

ROOT = "/home/pheno/Projects/neo-code"
WORK = tempfile.mkdtemp(prefix="s5-smoke-")
orig_home = os.environ.get("HOME", "")
# 网关与客户端运行时共享临时 HOME（token 存储隔离）；
# go build 仍用原 HOME（模块缓存离线可用，构建与运行环境解耦）。
os.environ["HOME"] = WORK

print(f"[smoke] workdir: {WORK}")

# 1) 构建两个二进制
build_env = os.environ.copy()
build_env["HOME"] = orig_home
os.environ["HOME"] = WORK  # 运行时 HOME 恢复为临时目录（token 存储隔离）
for target, out in [("neocode-gateway", f"{WORK}/neocode-gateway"), ("neocode-tuiv2", f"{WORK}/neocode-tuiv2")]:
    r = subprocess.run(["go", "build", "-mod=mod", "-o", out, f"./cmd/{target}"], cwd=ROOT, capture_output=True, env=build_env)
    if r.returncode != 0:
        print(f"FAIL build {target}: {r.stderr.decode()[:500]}")
        sys.exit(1)
print("[smoke] binaries built")

# 2) 拉起网关（随机端口）
# 网关默认在 $HOME/.neocode/run/gateway.sock 建 IPC socket；
# v1 RPC 客户端是 IPC-only 传输（unix socket/命名管道），共享 HOME 即零配置互通。
gateway = subprocess.Popen(
    [f"{WORK}/neocode-gateway"],
    stdout=subprocess.PIPE, stderr=subprocess.STDOUT, env=os.environ.copy(),
)
sock = os.path.join(WORK, ".neocode", "run", "gateway.sock")
deadline = time.time() + 15
while time.time() < deadline and not os.path.exists(sock):
    time.sleep(0.2)
if not os.path.exists(sock):
    print("FAIL gateway IPC socket not created")
    gateway.terminate()
    sys.exit(1)
print(f"[smoke] gateway ipc at {sock}")

failures = []
try:
    # 3) tuiv2 真实后端 pty 启动
    pid, fd = pty.fork()
    if pid == 0:
        os.environ["TERM"] = "xterm-256color"
        os.execvp(f"{WORK}/neocode-tuiv2", [
            f"{WORK}/neocode-tuiv2",
            "--backend=gateway",
        ])
        os._exit(127)

    buf = b""
    def respond(chunk):
        if b"\x1b]11;?" in chunk: os.write(fd, b"\x1b]11;rgb:0000/0000/0000\x1b\\")
        if b"\x1b[6n" in chunk: os.write(fd, b"\x1b[1;1R")
    def drain(t=0.8):
        global buf
        end = time.time() + t
        while time.time() < end:
            r, _, _ = select.select([fd], [], [], 0.2)
            if r:
                try: d = os.read(fd, 65536)
                except OSError: return False
                if not d: return False
                buf += d; respond(d)
        return True

    time.sleep(3.0); drain(1.5)
    frame = re.sub(rb"\x1b\[[0-9;]*m", b"", buf).decode("utf-8", "replace")
    ok_frame = "NEOCODE" in frame
    failures.append(("初始帧渲染（真实网关认证+bootstrap 链路）", ok_frame))

    # 建会话入口（space→Leader→s 或 :new 经 Ex 行）——用 Ex 行 :new 触发 createSession RPC
    os.write(fd, b"\x1b"); time.sleep(0.4)   # esc → Normal
    os.write(fd, b":"); time.sleep(0.3)      # Ex 行
    os.write(fd, b"new"); time.sleep(0.3)
    os.write(fd, b"\r"); time.sleep(1.5); drain(1.0)
    after = re.sub(rb"\x1b\[[0-9;]*m", b"", buf).decode("utf-8", "replace")
    # createSession RPC 成功与否的可见证据：失败会 Notify 错误文案（含 "error"/"失败"）
    ok_new = not re.search(r"(error|失败|Error)", after.split("new")[-1])
    failures.append((":new 创建会话链路无错误提示", ok_new))

    # 退出
    os.write(fd, b"\x1b"); time.sleep(0.3)
    os.write(fd, b":"); time.sleep(0.3)
    os.write(fd, b"q"); time.sleep(0.3)
    os.write(fd, b"\r")
    exit_code = None
    deadline = time.time() + 10
    while time.time() < deadline:
        done, status = os.waitpid(pid, os.WNOHANG)
        if done:
            exit_code = os.waitstatus_to_exitcode(status)
            break
        r, _, _ = select.select([fd], [], [], 0.3)
        if r:
            try:
                d = os.read(fd, 65536)
                buf += d
                if not d:
                    done, status = os.waitpid(pid, os.WNOHANG)
                    if done:
                        exit_code = os.waitstatus_to_exitcode(status)
                        break
                    deadline = time.time() + 2
            except OSError:
                done, status = os.waitpid(pid, os.WNOHANG)
                if done:
                    exit_code = os.waitstatus_to_exitcode(status)
                break
    failures.append((":q 退出（真实网关路径）", exit_code == 0))
finally:
    if gateway.poll() is None:
        gateway.terminate()
        try: gateway.wait(5)
        except Exception: gateway.kill()

for name, ok in failures:
    print(f"{'PASS' if ok else 'FAIL'}  {name}")
sys.exit(0 if all(ok for _, ok in failures) else 1)
