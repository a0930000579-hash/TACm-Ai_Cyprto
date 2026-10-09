p='run_defi_smoke.sh'
s=open(p).read()
old_start = s.index('echo "$P" | grep -q')
old_end = s.index('|| bad "pools:', old_start)
old_end = s.index('\n', s.index('"head -c 200)"', old_end))
line_end = s.index('\n', old_end)
old = s[old_start:line_end]
new = '''echo "$P" | $PY -c "import sys,json; d=json.load(sys.stdin); ps=[p['pair'] for p in d['pools']]; assert ('TACM/USDT' in ps) and ('TIUSD/USDT' in ps) and ('TACM/TIUSD' in ps or 'TACM/TiUSD' in ps), ps" \\
  && ok "流動性池 seed 3 池（TACM/USDT、TACM/TIUSD、TIUSD/USDT）" || bad "pools: $(echo "$P" | head -c 200)"'''
s = s[:old_start] + new + s[line_end:]
open(p,'w').write(s)
print('patched')
