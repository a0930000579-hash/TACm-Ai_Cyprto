package vm

import "math/big"

// 本檔提供「鏈上標準代幣（ERC-20 風格）」的完整位元組碼與輔助函數，
// 讓任何地址可在 TAC 鏈上部署自有代幣（totalSupply 由部署者於構造時指定，
// 初始供應量全部結給部署者），並支援 totalSupply / balanceOf / transfer /
// name / symbol 五項標準介面（M43＋M44）。
//
// Storage 配置（標準 EVM slot 模型）：
//
//	slot0                = totalSupply
//	slot1                = name（右對齊 32-byte）
//	slot2                = symbol（右對齊 32-byte）
//	slot[address]        = balance（以 256-bit 整數為 key）
//
// Runtime 採用 selector dispatch（CALLDATALOAD(0) 後 DIV 2^224 只取前
// 4-byte selector 與常數比較），比中後 JUMP 至對應實作。

// ERC-20 標準 function selector（4-byte，右對齊於 32-byte 字）。
var (
	SelectorTotalSupply = []byte{0x18, 0x16, 0x0d, 0xdd} // totalSupply()
	SelectorBalanceOf   = []byte{0x70, 0xa0, 0x82, 0x31} // balanceOf(address)
	SelectorTransfer    = []byte{0xa9, 0x05, 0x9c, 0xbb} // transfer(address,uint256)
	SelectorName        = []byte{0x06, 0xfd, 0xde, 0x03} // name()
	SelectorSymbol      = []byte{0x95, 0xd8, 0x9b, 0x41} // symbol()
)

// RET32 為「值 MSTORE@0、RETURN(0,32)」尾碼。
var RET32 = []byte{0x60, 0x00, 0x52, 0x60, 0x20, 0x60, 0x00, 0xF3}

// push32 產生 PUSH32 <32-byte 右對齊值>。
func push32(x *big.Int) []byte {
	out := []byte{0x7F}
	return append(out, IntToBytes(x, 32)...)
}

// selNum 把 4-byte selector 轉為右對齊數值（DIV 2^224 後可直接 EQ）。
func selNum(sel []byte) *big.Int {
	return new(big.Int).SetBytes(sel)
}

// Erc20Runtime 回傳標準 ERC-20 runtime 位元組碼（固定）。
// selector 解析：CALLDATALOAD(0) 後 DIV 2^224 只取前 4 bytes（避免參數
// 污染 32-byte 比較）；每個比較前 DUP1 保留 sel4；JUMPI 棧頂 dest、次 cond。
// 跳轉目標 >255 者使用 PUSH2（0x61）。
//
//	[0]   60 00 35             CALLDATALOAD(0) → sel32
//	[3]   7F 0x1<<224          PUSH32(2^224)
//	[36]  04                   DIV → sel4（前 4 bytes）
//	[37]  80                   DUP1
//	[38]  7F 0x18160ddd        PUSH32 totalSupply selector
//	[71]  14                   EQ
//	[72]  60 EB 57             JUMPI → d1(235)
//	[75]  80                   DUP1
//	[76]  7F 0x70a08231        PUSH32 balanceOf selector
//	[109] 14                   EQ
//	[110] 60 F7 57             JUMPI → d2(247)
//	[113] 80                   DUP1
//	[114] 7F 0xa9059cbb        PUSH32 transfer selector
//	[147] 14                   EQ
//	[148] 61 01 04 57          JUMPI → d3(260)
//	[152] 80                   DUP1
//	[153] 7F 0x06fdde03        PUSH32 name selector
//	[186] 14                   EQ
//	[187] 61 01 37 57          JUMPI → d4(311)
//	[191] 80                   DUP1
//	[192] 7F 0x95d89b41        PUSH32 symbol selector
//	[225] 14                   EQ
//	[226] 61 01 43 57          JUMPI → d5(323)
//	[230] 60 00 60 00 F3       fallback RETURN(0,0)
//	[235] 5B                   d1: totalSupply → SLOAD slot0 → RETURN(0,32)
//	[247] 5B                   d2: balanceOf → CALLDATALOAD(4) SLOAD → RETURN(0,32)
//	[260] 5B                   d3: transfer（M47 加餘額檢查：bal<amt → REVERT）
//	[311] 5B                   d4: name → SLOAD slot1 → RETURN(0,32)
//	[323] 5B                   d5: symbol → SLOAD slot2 → RETURN(0,32)
func Erc20Runtime() []byte {
	var b []byte
	// dispatch
	b = append(b, 0x60, 0x00, 0x35) // CALLDATALOAD(0)
	b = append(b, push32(new(big.Int).Lsh(big.NewInt(1), 224))...) // 2^224
	b = append(b, 0x04)             // DIV → sel4
	b = append(b, 0x80)             // DUP1
	b = append(b, push32(selNum(SelectorTotalSupply))...)
	b = append(b, 0x14)            // EQ
	b = append(b, 0x60, 235, 0x57) // JUMPI d1
	b = append(b, 0x80)            // DUP1
	b = append(b, push32(selNum(SelectorBalanceOf))...)
	b = append(b, 0x14)
	b = append(b, 0x60, 247, 0x57) // JUMPI d2
	b = append(b, 0x80)            // DUP1
	b = append(b, push32(selNum(SelectorTransfer))...)
	b = append(b, 0x14)
	b = append(b, 0x61, 0x01, 0x04, 0x57) // JUMPI d3(260) PUSH2
	b = append(b, 0x80)                  // DUP1
	b = append(b, push32(selNum(SelectorName))...)
	b = append(b, 0x14)
	b = append(b, 0x61, 0x01, 0x37, 0x57) // JUMPI d4(311) PUSH2
	b = append(b, 0x80)                  // DUP1
	b = append(b, push32(selNum(SelectorSymbol))...)
	b = append(b, 0x14)
	b = append(b, 0x61, 0x01, 0x43, 0x57) // JUMPI d5(323) PUSH2
	b = append(b, 0x60, 0x00, 0x60, 0x00, 0xF3) // fallback RETURN(0,0)

	// d1: totalSupply
	b = append(b, 0x5B)          // JUMPDEST
	b = append(b, 0x60, 0x00, 0x54) // SLOAD slot0
	b = append(b, RET32...)

	// d2: balanceOf
	b = append(b, 0x5B)          // JUMPDEST
	b = append(b, 0x60, 0x04, 0x35) // CALLDATALOAD(4) → holder
	b = append(b, 0x54)          // SLOAD slot[holder]
	b = append(b, RET32...)

	// d3: transfer(to, amount) — 含餘額檢查：bal < amt 則 REVERT(0,0)，
	// 防止 uint256 下溢（from 餘額不足不得扣款）。
	// 檢查段消耗 bal/amt 後，ok 段重載 caller 餘額與金額再扣款／加款。
	b = append(b, 0x5B)          // JUMPDEST (260)
	b = append(b, 0x60, 0x04, 0x35) // to = CALLDATALOAD(4)
	b = append(b, 0x33)          // from = CALLER
	b = append(b, 0x54)          // bal = SLOAD slot[from]
	b = append(b, 0x60, 0x24, 0x35) // amt = CALLDATALOAD(36)
	b = append(b, 0x10)          // LT → bal<amt（棧：to from cond）
	b = append(b, 0x15)          // ISZERO → ok
	b = append(b, 0x61, 0x01, 0x18, 0x57) // JUMPI → ok(280)
	b = append(b, 0x60, 0x00, 0x60, 0x00, 0xFD) // REVERT(0,0)
	b = append(b, 0x5B)          // ok: JUMPDEST (280)；棧：to from
	b = append(b, 0x33)          // CALLER from
	b = append(b, 0x54)          // SLOAD slot[from] bal
	b = append(b, 0x60, 0x24, 0x35) // amt
	b = append(b, 0x03)          // SUB → newFrom
	b = append(b, 0x33)          // CALLER
	b = append(b, 0x55)          // SSTORE slot[caller]=newFrom
	b = append(b, 0x60, 0x04, 0x35) // to
	b = append(b, 0x54)          // SLOAD slot[to]
	b = append(b, 0x60, 0x24, 0x35) // amt
	b = append(b, 0x01)          // ADD → newTo
	b = append(b, 0x60, 0x04, 0x35) // to
	b = append(b, 0x55)          // SSTORE slot[to]=newTo
	b = append(b, 0x60, 0x01)    // PUSH1 1
	b = append(b, RET32...)

	// d4: name
	b = append(b, 0x5B)          // JUMPDEST
	b = append(b, 0x60, 0x01, 0x54) // SLOAD slot1
	b = append(b, RET32...)

	// d5: symbol
	b = append(b, 0x5B)          // JUMPDEST
	b = append(b, 0x60, 0x02, 0x54) // SLOAD slot2
	b = append(b, RET32...)
	return b
}

// Erc20Init 回傳部署位元組碼（無 name/symbol 元資料）：把 totalSupply 寫入
// slot0、並將總供應量全部結給部署者（CALLER=creator），最後回傳 runtime。
func Erc20Init(totalSupply *big.Int) []byte {
	return Erc20InitWithMeta(totalSupply, nil, nil)
}

// Erc20InitWithMeta 回傳帶元資料（name/symbol）的部署位元組碼：額外把
// name 寫入 slot1、symbol 寫入 slot2（皆為右對齊 32-byte）。
func Erc20InitWithMeta(totalSupply *big.Int, name, symbol []byte) []byte {
	runtime := Erc20Runtime()
	var b []byte
	// slot0 = totalSupply（SSTORE：key 棧頂，先壓 value 再壓 key 0）
	b = append(b, push32(totalSupply)...)
	b = append(b, 0x60, 0x00) // key 0
	b = append(b, 0x55)       // SSTORE
	// slot1 = name（右對齊）
	b = append(b, push32(new(big.Int).SetBytes(name))...)
	b = append(b, 0x60, 0x01) // key 1
	b = append(b, 0x55)       // SSTORE
	// slot2 = symbol（右對齊）
	b = append(b, push32(new(big.Int).SetBytes(symbol))...)
	b = append(b, 0x60, 0x02) // key 2
	b = append(b, 0x55)       // SSTORE
	// slot[creator] = totalSupply（key 棧頂：先壓 value=supply、再壓 key=creator）
	b = append(b, push32(totalSupply)...)
	b = append(b, 0x33) // CALLER → creator（key 頂）
	b = append(b, 0x55) // SSTORE
	// CODECOPY(runtime) + RETURN(runtime)：runtime 位於 init 指令段
	// （CODECOPY 段 9 bytes + RETURN 段 6 bytes，size/src 用 PUSH2）
	// 之後，src = initLen + 15。
	initLen := len(b)
	rl := len(runtime)
	src := initLen + 15
	b = append(b, 0x61, byte(rl>>8), byte(rl&0xFF))      // PUSH2 size
	b = append(b, 0x61, byte(src>>8), byte(src&0xFF))    // PUSH2 src
	b = append(b, 0x60, 0x00)                            // PUSH dest
	b = append(b, 0x39)                                  // CODECOPY（dest 棧頂）
	b = append(b, 0x61, byte(rl>>8), byte(rl&0xFF), 0x60, 0x00, 0xF3) // RETURN(0, rl)
	return append(b, runtime...)
}

// Erc20BalanceOfCalldata 組裝 balanceOf(address) calldata。
func Erc20BalanceOfCalldata(holder0x string) []byte {
	out := append([]byte{}, SelectorBalanceOf...)
	return append(out, IntToBytes(addrToInt(holder0x), 32)...)
}

// Erc20TransferCalldata 組裝 transfer(to, amount) calldata。
func Erc20TransferCalldata(to0x string, amount *big.Int) []byte {
	out := append([]byte{}, SelectorTransfer...)
	out = append(out, IntToBytes(addrToInt(to0x), 32)...)
	return append(out, IntToBytes(amount, 32)...)
}

// Erc20TotalSupplyCalldata 組裝 totalSupply() calldata。
func Erc20TotalSupplyCalldata() []byte {
	return append([]byte{}, SelectorTotalSupply...)
}

// Erc20NameCalldata 組裝 name() calldata。
func Erc20NameCalldata() []byte {
	return append([]byte{}, SelectorName...)
}

// Erc20SymbolCalldata 組裝 symbol() calldata。
func Erc20SymbolCalldata() []byte {
	return append([]byte{}, SelectorSymbol...)
}
