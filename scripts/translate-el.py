#!/usr/bin/env python3
"""Machine-translate el.po (English -> Greek) with placeholders protected."""
import re, json, sys, time, urllib.parse, urllib.request

PO = "src/i18n/locales/el.po"

# Force these regardless of MT output: word-sense errors ("logs" = tree trunks)
# and domain terms the machine gets wrong. Keyed by the English msgid.
GLOSSARY = {
    "Logs": "Αρχεία καταγραφής",
    "Branch Logic": "Λογική διακλάδωσης",
    "Host": "Διακομιστής",
    "Hosts": "Διακομιστές",
    "Contact": "Επαφή",
    "Condition": "Συνθήκη",
    "Conditions": "Συνθήκες",
    "State": "Κατάσταση",
    "States": "Καταστάσεις",
    "Event": "Συμβάν",
    "Events": "Συμβάντα",
}

def dec(x): return x.replace('\\"', '"').replace("\\\\", "\\")
def enc(x): return x.replace("\\", "\\\\").replace('"', '\\"')

def extract_msgid(lines):
    cur = None; parts = []
    for l in lines:
        m = re.match(r'^msgid "(.*)"\s*$', l)
        if m: cur = "id"; parts = [dec(m.group(1))]; continue
        m = re.match(r'^msgctxt "(.*)"\s*$', l)
        if m: cur = "ctx"; continue
        if cur == "id" and l.startswith('"') and l.endswith('"'):
            parts.append(dec(l[1:-1]))
    return "".join(parts)

def get_msgstr_region(lines):
    for i, l in enumerate(lines):
        if l.startswith('msgstr "') or l.startswith('msgstr['):
            j = i
            while j + 1 < len(lines) and lines[j + 1].startswith('"'):
                j += 1
            return i, j
    return None, None

class Protector:
    def __init__(self):
        self.tokens = {}
        self.used = set()
    def protect(self, t):
        self.used = set()
        def rep(m):
            s = m.group(0)
            if s not in self.tokens:
                self.tokens[s] = f"\u27E6T{len(self.tokens)}\u27E7"
            tok = self.tokens[s]
            self.used.add(tok)
            return tok
        t = re.sub(r"\{[^{}]*\}", rep, t)
        t = re.sub(r"%(\d+\$)?[sdfx%]", rep, t)
        t = re.sub(r"<[^>]+>", rep, t)
        t = t.replace("\n", "\u27E6NL\u27E7")
        return t
    def restore(self, t):
        for tok, src in self.tokens.items():
            t = t.replace(tok, "\n" if src == "\n" else src)
        return t

def translate_batch(msgs):
    out = {}
    chunk = 100
    for start in range(0, len(msgs), chunk):
        part = msgs[start:start + chunk]
        payload = "\n".join(part)
        got = None
        for attempt in range(3):
            try:
                q = urllib.parse.urlencode({"client": "gtx", "sl": "en", "tl": "el", "dt": "t", "q": payload})
                req = urllib.request.Request("https://translate.googleapis.com/translate_a/single?" + q,
                                             headers={"User-Agent": "Mozilla/5.0"})
                got = json.load(urllib.request.urlopen(req, timeout=30))[0]
                break
            except Exception as e:
                print(f"  batch {start//chunk} retry {attempt+1}: {type(e).__name__}", file=sys.stderr)
                time.sleep(1.5)
        if not got:
            continue
        if len(got) == len(part):
            for expected, seg in zip(part, got):
                out[expected] = (seg[0] or "").rstrip("\n")
        else:
            for seg in got:
                orig = (seg[1] or "").rstrip("\n")
                if orig in part:
                    out[orig] = (seg[0] or "").rstrip("\n")
        time.sleep(0.2)
    return out

text = open(PO, encoding="utf-8").read()
blocks = text.split("\n\n")
prot = Protector()
block_info = []
ids = []
for blk in blocks:
    lines = blk.split("\n")
    msgid = extract_msgid(lines)
    if msgid == "":
        block_info.append((lines, "", set()))
        continue
    p = prot.protect(msgid)
    used = set(prot.used)
    block_info.append((lines, p, used, msgid))
    ids.append(p)

unique = sorted(set(p for p in ids if p))
print(f"unique msgids: {len(unique)}", file=sys.stderr)
translations = translate_batch(unique)
print(f"translations: {len(translations)}", file=sys.stderr)

done = 0
out_blocks = []
for lines, pid, used, msgid in block_info:
    if pid == "":
        out_blocks.append("\n".join(lines))
        continue
    tr = translations.get(pid)
    if msgid in GLOSSARY:
        tr = GLOSSARY[msgid]
    if tr is not None:
        missing = [tok for tok in used if tok not in tr]
        if missing:
            tr = None
    out_lines = list(lines)
    i, j = get_msgstr_region(out_lines)
    if i is None:
        out_blocks.append("\n".join(lines))
        continue
    if tr is not None:
        restored = prot.restore(tr)
        out_lines[i] = f'msgstr "{enc(restored)}"'
        for k in range(i + 1, j + 1):
            out_lines[k] = ""
        done += 1
    else:
        out_lines[i] = 'msgstr ""'
        for k in range(i + 1, j + 1):
            out_lines[k] = ""
    out_blocks.append("\n".join(out_lines))

open(PO, "w", encoding="utf-8").write("\n\n".join(out_blocks))
print(f"translated {done} / {len([b for b in block_info if b[1]])}", file=sys.stderr)
