package vm

import "math/big"

// 本檔提供「鏈上標準 NFT（ERC-721 風格）」的完整位元組碼與輔助函數，
// 讓任何地址可在 TAC 鏈上部署自有 NFT 集合，支援標準介面與 Transfer 事件
// （topic0 與乙太坊/BSC 主網一致），向主流鏈 NFT 標準看齊（M74-2）。
//
// Storage 配置（標準 EVM slot 模型）：
//
//	slot0                       = totalSupply（已 mint 數）
//	slot1                       = name（右對齊 32-byte）
//	slot2                       = symbol（右對齊 32-byte）
//	ownerOf(tokenId)            = keccak(tokenId ‖ 3)（mapping base=3）
//	balanceOf(owner)            = keccak(owner ‖ 4)（mapping base=4）
//	tokenURI(tokenId)           = keccak(tokenId ‖ 5)（mapping base=5）
//
// 支援介面（與乙太坊/BSC 標準一致）：
//   - totalSupply() / name() / symbol()
//   - balanceOf(address) / ownerOf(uint256)
//   - mint(address,uint256)（無重複 mint 檢查、Transfer(0,to,tokenId) 事件）
//   - transferFrom(address,address,uint256)（僅 owner 本人可轉、Transfer 事件）
//   - tokenURI(uint256)（讀 mapping，未設定回空）

// ERC-721 標準 function selector（4-byte）。
var (
	Selector721BalanceOf   = []byte{0x70, 0xa0, 0x82, 0x31} // balanceOf(address)
	Selector721OwnerOf     = []byte{0x63, 0x52, 0x21, 0x1e} // ownerOf(uint256)
	Selector721TransferFrom = []byte{0x23, 0xb8, 0x72, 0xdd} // transferFrom(address,address,uint256)
	Selector721Mint        = []byte{0x40, 0xc1, 0x0f, 0x19} // mint(address,uint256)
	Selector721TokenURI    = []byte{0xc8, 0x7b, 0x56, 0xdd} // tokenURI(uint256)
	Selector721TotalSupply = []byte{0x18, 0x16, 0x0d, 0xdd} // totalSupply()
	Selector721Name        = []byte{0x06, 0xfd, 0xde, 0x03} // name()
	Selector721Symbol      = []byte{0x95, 0xd8, 0x9b, 0x41} // symbol()
	// ERC-721 的 Transfer 事件 topic0 與 ERC-20 相同（Transfer(address,address,uint256)）。
	Topic721Transfer = TopicTransfer
)

// mapSlotCode 產生「計算 mapping slot（keccak(key ‖ base)）」的 bytecode 片段：
// mem[0]=key、mem[32]=base → SHA3(0,64)（棧頂回傳 slot）。
func mapSlotCode(keyCode []byte, base byte) []byte {
	var b []byte
	b = append(b, keyCode...)
	b = append(b, 0x60, 0x00, 0x52)                 // MSTORE(0, key)
	b = append(b, 0x60, base, 0x60, 0x20, 0x52)    // MSTORE(32, base)
	b = append(b, 0x60, 0x40, 0x60, 0x00, 0x20)    // SHA3(0,64) → slot
	return b
}

// Erc721Runtime 回傳標準 ERC-721 runtime 位元組碼（固定）。
// selector 解析與 Erc20Runtime 相同（CALLDATALOAD(0)→DIV 2^224→DUP1→EQ→JUMPI）。
func Erc721Runtime() []byte {
	selOrder := [][]byte{
		Selector721TotalSupply, Selector721BalanceOf, Selector721OwnerOf,
		Selector721Name, Selector721Symbol, Selector721Mint,
		Selector721TransferFrom, Selector721TokenURI,
	}
	header := []byte{0x60, 0x00, 0x35} // CALLDATALOAD(0)
	header = append(header, push32(new(big.Int).Lsh(big.NewInt(1), 224))...)
	header = append(header, 0x04) // DIV → sel4
	type jumpPatch struct{ pos, label int }
	var patches []jumpPatch
	d := append([]byte{}, header...)
	for i, sel := range selOrder {
		d = append(d, 0x80) // DUP1
		d = append(d, push32(selNum(sel))...)
		d = append(d, 0x14, 0x61, 0x00, 0x00, 0x57) // EQ / PUSH2 dst / JUMPI
		patches = append(patches, jumpPatch{pos: len(d) - 3, label: i})
	}
	d = append(d, 0x60, 0x00, 0x60, 0x00, 0xF3) // fallback RETURN(0,0)

	segs := []builtSeg{
		erc721SegTotalSupply(), erc721SegBalanceOf(), erc721SegOwnerOf(),
		erc721SegName(), erc721SegSymbol(), erc721SegMint(),
		erc721SegTransferFrom(), erc721SegTokenURI(),
	}
	labs := make([]int, len(segs))
	off := len(d)
	for i, sg := range segs {
		labs[i] = off
		off += len(sg.code)
	}
	for _, p := range patches {
		d[p.pos] = byte(labs[p.label] >> 8)
		d[p.pos+1] = byte(labs[p.label] & 0xFF)
	}
	for i, sg := range segs {
		for _, j := range sg.jumps {
			abs := labs[i] + j.target
			sg.code[j.pos] = byte(abs >> 8)
			sg.code[j.pos+1] = byte(abs & 0xFF)
		}
	}
	for _, sg := range segs {
		d = append(d, sg.code...)
	}
	return d
}

// erc721SegTotalSupply: totalSupply → SLOAD slot0 → RETURN(0,32)
func erc721SegTotalSupply() builtSeg {
	b := []byte{0x5B, 0x60, 0x00, 0x54}
	return builtSeg{code: append(b, RET32...)}
}

// erc721SegBalanceOf: balanceOf(owner) → mapping(owner,4) → SLOAD → RETURN(0,32)
func erc721SegBalanceOf() builtSeg {
	b := []byte{0x5B}
	b = append(b, mapSlotCode(codeCalldata4, 4)...)
	b = append(b, 0x54)
	return builtSeg{code: append(b, RET32...)}
}

// erc721SegOwnerOf: ownerOf(tokenId) → mapping(tokenId,3) → SLOAD → RETURN(0,32)
func erc721SegOwnerOf() builtSeg {
	b := []byte{0x5B}
	b = append(b, mapSlotCode(codeCalldata4, 3)...)
	b = append(b, 0x54)
	return builtSeg{code: append(b, RET32...)}
}

// erc721SegName: name → SLOAD slot1 → RETURN(0,32)
func erc721SegName() builtSeg {
	b := []byte{0x5B, 0x60, 0x01, 0x54}
	return builtSeg{code: append(b, RET32...)}
}

// erc721SegSymbol: symbol → SLOAD slot2 → RETURN(0,32)
func erc721SegSymbol() builtSeg {
	b := []byte{0x5B, 0x60, 0x02, 0x54}
	return builtSeg{code: append(b, RET32...)}
}

// erc721SegMint: mint(to, tokenId)——require ownerOf(tokenId)==0（不可重複）、
// 寫 ownerOf[tokenId]=to、balance[to]++、totalSupply++、
// LOG3(Transfer, 0, to, tokenId)、回傳 1。
func erc721SegMint() builtSeg {
	var b []byte
	var jumps []segJump
	b = append(b, 0x5B) // JUMPDEST
	b = append(b, mapSlotCode(codeCalldata24, 3)...) // ownerSlot（tokenId=calldata[36]）
	b = append(b, 0x80) // DUP1
	b = append(b, 0x54) // owner
	b = append(b, 0x15) // ISZERO（owner==0 才可 mint）
	okPos := len(b) + 1
	b = append(b, 0x61, 0x00, 0x00, 0x57) // JUMPI ok
	jumps = append(jumps, segJump{pos: okPos, target: okPos + 8})
	b = append(b, 0x60, 0x00, 0x60, 0x00, 0xFD) // REVERT
	b = append(b, 0x5B)                       // ok: [ownerSlot]
	// ownerOf[tokenId]=to（SWAP1 後 ownerSlot 回棧頂當 key）
	b = append(b, codeCalldata4...)           // to
	b = append(b, 0x90)                       // SWAP1 → [ownerSlot, to]
	b = append(b, 0x55)                       // SSTORE ownerOf[tokenId]=to
	// balance[to]++：balSlot=map(to,4)，DUP1 SLOAD +1，SSTORE（balSlot 棧頂）
	b = append(b, mapSlotCode(codeCalldata4, 4)...)
	b = append(b, 0x80)                       // DUP1
	b = append(b, 0x54)                       // bal
	b = append(b, 0x60, 0x01, 0x01)           // bal+1（ADD：1 頂、bal 次）
	b = append(b, 0x90)                       // SWAP1 → [bal+1, balSlot]（key 回頂）
	b = append(b, 0x55)                       // SSTORE bal[to]=bal+1
	// totalSupply++
	b = append(b, 0x60, 0x00, 0x54)           // supply
	b = append(b, 0x60, 0x01, 0x01)           // supply+1
	b = append(b, 0x60, 0x00, 0x55)           // SSTORE slot0
	// LOG3(Transfer, 0, to, tokenId)
	b = append(b, codeCalldata24...)          // tokenId（data）
	b = append(b, 0x60, 0x00, 0x52)           // mem[0]=tokenId
	b = append(b, push32(selNum(Topic721Transfer))...)
	b = append(b, 0x60, 0x00)                 // topic2=0（from=ZeroAddress）
	b = append(b, codeCalldata4...)           // to（topic3）
	b = append(b, codeLog3Tail...)
	b = append(b, codeReturnTrue...)
	b = append(b, RET32...)
	return builtSeg{code: b, jumps: jumps}
}

// erc721SegTransferFrom: transferFrom(from, to, tokenId)——require ownerOf(tokenId)==from
// 且 caller==from（僅 owner 本人可轉）、ownerOf[tokenId]=to、balance[from]--、balance[to]++、
// LOG3(Transfer, from, to, tokenId)、回傳 1。
func erc721SegTransferFrom() builtSeg {
	var b []byte
	var jumps []segJump
	// 1) owner 檢查：ownerOf(tokenId)==from
	b = append(b, 0x5B) // JUMPDEST
	b = append(b, mapSlotCode(codeCalldata68, 3)...) // ownerSlot（tokenId=calldata[68]）
	b = append(b, 0x80) // DUP1
	b = append(b, 0x54) // owner
	b = append(b, codeCalldata4...) // from
	b = append(b, 0x14) // EQ（pop from、owner → from==owner）
	ok1Pos := len(b) + 1
	b = append(b, 0x61, 0x00, 0x00, 0x57) // JUMPI ok1
	jumps = append(jumps, segJump{pos: ok1Pos, target: ok1Pos + 8})
	b = append(b, 0x60, 0x00, 0x60, 0x00, 0xFD) // REVERT
	b = append(b, 0x5B)                       // ok1: [ownerSlot]
	// 2) caller 檢查：caller==from
	b = append(b, codeCalldata4...)           // from
	b = append(b, 0x33)                       // caller
	b = append(b, 0x14)                       // EQ
	ok2Pos := len(b) + 1
	b = append(b, 0x61, 0x00, 0x00, 0x57)     // JUMPI ok2
	jumps = append(jumps, segJump{pos: ok2Pos, target: ok2Pos + 8})
	b = append(b, 0x60, 0x00, 0x60, 0x00, 0xFD) // REVERT
	b = append(b, 0x5B)                       // ok2: [ownerSlot]
	// 3) ownerOf[tokenId]=to（SWAP1 後 ownerSlot 回棧頂當 key）
	b = append(b, codeCalldata24...)          // to
	b = append(b, 0x90)                       // SWAP1 → [ownerSlot, to]
	b = append(b, 0x55)                       // SSTORE ownerOf[tokenId]=to
	// 4) balance[from]--：balSlot=map(from,4)，DUP1 SLOAD -1，SSTORE
	b = append(b, mapSlotCode(codeCalldata4, 4)...)
	b = append(b, 0x80)                       // DUP1
	b = append(b, 0x54)                       // bal
	b = append(b, 0x60, 0x01, 0x03)           // bal-1（SUB：1 頂、bal 次）
	b = append(b, 0x90)                       // SWAP1 → [bal-1, balSlot]（key 回頂）
	b = append(b, 0x55)                       // SSTORE bal[from]=bal-1
	// 5) balance[to]++：balSlot=map(to,4)
	b = append(b, mapSlotCode(codeCalldata24, 4)...)
	b = append(b, 0x80)                       // DUP1
	b = append(b, 0x54)                       // bal
	b = append(b, 0x60, 0x01, 0x01)           // bal+1
	b = append(b, 0x90)                       // SWAP1 → [bal+1, balSlot]（key 回頂）
	b = append(b, 0x55)                       // SSTORE bal[to]=bal+1
	// 6) LOG3(Transfer, from, to, tokenId)
	b = append(b, codeCalldata68...)          // tokenId（data）
	b = append(b, 0x60, 0x00, 0x52)           // mem[0]=tokenId
	b = append(b, push32(selNum(Topic721Transfer))...)
	b = append(b, codeCalldata4...)           // from（topic2）
	b = append(b, codeCalldata24...)          // to（topic3）
	b = append(b, codeLog3Tail...)
	b = append(b, codeReturnTrue...)
	b = append(b, RET32...)
	return builtSeg{code: b, jumps: jumps}
}

// erc721SegTokenURI: tokenURI(tokenId) → mapping(tokenId,5) → SLOAD → RETURN(0,32)
func erc721SegTokenURI() builtSeg {
	b := []byte{0x5B}
	b = append(b, mapSlotCode(codeCalldata4, 5)...)
	b = append(b, 0x54)
	return builtSeg{code: append(b, RET32...)}
}

// Erc721InitWithMeta 回傳帶元資料（name/symbol）的部署位元組碼：把 name 寫入
// slot1、symbol 寫入 slot2（右對齊 32-byte）、回傳 runtime。
func Erc721InitWithMeta(name, symbol []byte) []byte {
	runtime := Erc721Runtime()
	var b []byte
	b = append(b, push32(new(big.Int).SetBytes(name))...)
	b = append(b, 0x60, 0x01) // key 1
	b = append(b, 0x55)       // SSTORE
	b = append(b, push32(new(big.Int).SetBytes(symbol))...)
	b = append(b, 0x60, 0x02) // key 2
	b = append(b, 0x55)       // SSTORE
	initLen := len(b)
	rl := len(runtime)
	src := initLen + 15
	b = append(b, 0x61, byte(rl>>8), byte(rl&0xFF))                   // PUSH2 size
	b = append(b, 0x61, byte(src>>8), byte(src&0xFF))                 // PUSH2 src
	b = append(b, 0x60, 0x00)                                         // PUSH dest
	b = append(b, 0x39)                                               // CODECOPY（dest 棧頂）
	b = append(b, 0x61, byte(rl>>8), byte(rl&0xFF), 0x60, 0x00, 0xF3) // RETURN(0, rl)
	return append(b, runtime...)
}

// Erc721BalanceOfCalldata 組裝 balanceOf(address) calldata。
func Erc721BalanceOfCalldata(owner0x string) []byte {
	out := append([]byte{}, Selector721BalanceOf...)
	return append(out, IntToBytes(addrToInt(owner0x), 32)...)
}

// Erc721OwnerOfCalldata 組裝 ownerOf(uint256) calldata。
func Erc721OwnerOfCalldata(tokenID *big.Int) []byte {
	out := append([]byte{}, Selector721OwnerOf...)
	return append(out, IntToBytes(tokenID, 32)...)
}

// Erc721MintCalldata 組裝 mint(to, tokenId) calldata。
func Erc721MintCalldata(to0x string, tokenID *big.Int) []byte {
	out := append([]byte{}, Selector721Mint...)
	out = append(out, IntToBytes(addrToInt(to0x), 32)...)
	return append(out, IntToBytes(tokenID, 32)...)
}

// Erc721TransferFromCalldata 組裝 transferFrom(from, to, tokenId) calldata。
func Erc721TransferFromCalldata(from0x, to0x string, tokenID *big.Int) []byte {
	out := append([]byte{}, Selector721TransferFrom...)
	out = append(out, IntToBytes(addrToInt(from0x), 32)...)
	out = append(out, IntToBytes(addrToInt(to0x), 32)...)
	return append(out, IntToBytes(tokenID, 32)...)
}

// Erc721TokenURICalldata 組裝 tokenURI(uint256) calldata。
func Erc721TokenURICalldata(tokenID *big.Int) []byte {
	out := append([]byte{}, Selector721TokenURI...)
	return append(out, IntToBytes(tokenID, 32)...)
}

// Erc721TotalSupplyCalldata 組裝 totalSupply() calldata。
func Erc721TotalSupplyCalldata() []byte {
	return append([]byte{}, Selector721TotalSupply...)
}

// Erc721NameCalldata 組裝 name() calldata。
func Erc721NameCalldata() []byte {
	return append([]byte{}, Selector721Name...)
}

// Erc721SymbolCalldata 組裝 symbol() calldata。
func Erc721SymbolCalldata() []byte {
	return append([]byte{}, Selector721Symbol...)
}
