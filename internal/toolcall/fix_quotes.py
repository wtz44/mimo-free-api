import re

with open("toolparse.go", "r", encoding="utf-8") as f:
    content = f.read()

BT = chr(96)  # backtick

# Fix: replace regexp.MustCompile("...") with regexp.MustCompile(`...`)
def fix_re(m):
    inner = m.group(1)
    # Unescape \" back to "
    inner = inner.replace('\\"', '"')
    inner = inner.replace('\\\\', '\\')
    return f"regexp.MustCompile({BT}{inner}{BT})"

pattern = r'regexp\.MustCompile\("((?:[^"\\]|\\.)*)"\)'
content = re.sub(pattern, fix_re, content)

with open("toolparse.go", "w", encoding="utf-8") as f:
    f.write(content)

print("Fixed regex quotes")
