"""Fix toolparse.go to handle alt param format."""

fpath = "D:/mimo-gateway/internal/toolcall/toolparse.go"
with open(fpath, "r", encoding="utf-8") as f:
    c = f.read()

# Build XML tokens from char codes
p = chr(60) + "parameter"  # <parameter
p_close = chr(60) + chr(47) + "parameter" + chr(62)  # 