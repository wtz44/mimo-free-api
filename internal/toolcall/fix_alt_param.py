"""Fix toolparse.go to add <param=KEY> format support."""
import re

fpath = "D:/mimo-gateway/internal/toolcall/toolparse.go"
with open(fpath, "r", encoding="utf-8") as f:
    content = f.read()

LT = chr(60)
GT = chr(62)
SL = chr(47)
BT = chr(96)
P = "parameter"
EQ = chr(61)

# Add alt param regex after standardParamRe
# Pattern: <parameter=KEY>value