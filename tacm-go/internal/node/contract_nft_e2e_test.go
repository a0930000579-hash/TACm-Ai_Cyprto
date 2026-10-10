package node

import (
	"math/big"
	"strings"
	"testing"
	"time"

	"tacm/internal/crypto"
	"tacm/internal/vm"
)

// 本檔驗證 M74-2 鏈上 NFT（ERC-721 風格）RPC 閉環：
//  1. POST /contract/nft/deploy 部署 NFT 集合（name/symbol）
//  2. POST /contract/nft/mint 代簽 mint(to, tokenId)，入塊後 balance/ownerOf 變動
//  3. GET  /contract/nft/{addr} 便利查詢（name/symbol/supply/balance/owner）
//  4. POST /contract/nft/transfer 代簽 transferFrom，入塊後 owner 轉移
//  5. 重複 mint 同一 tokenId → REVERT（經只讀模擬呼叫驗證）

func TestContractNFTDeployMintTransfer(t *testing.T) {
	n, srv, admin := contractStudioSetup(t)
	bobKP, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	bob, _ := bobKP.Address()

	// 1. 部署 NFT 集合。
	out := studioPostJSON(t, srv, "/contract/nft/deploy", map[string]any{
		"name": "TAC Legends", "symbol": "TACL",
	})
	if out["ok"] != true {
		t.Fatalf("NFT 部署失敗: %v", out)
	}
	contractAddr := out["contract_address"].(string)
	contract0x, err := tx0To0x(contractAddr)
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool {
		info := n.contracts.Get(contract0x)
		return info != nil && info.CodeSize > 0
	}, 8*time.Second, "NFT 合約未入塊部署")

	// 2. 便利查詢初始狀態。
	info := studioGetJSON(t, srv, "/contract/nft/"+contractAddr)
	if info["ok"] != true {
		t.Fatalf("NFT 查詢失敗: %v", info)
	}
	if info["name"] != "TAC Legends" || info["symbol"] != "TACL" {
		t.Fatalf("NFT 元資料異常: %v", info)
	}
	if info["supply"] != "0" {
		t.Fatalf("初始 supply 應為 0，得到 %v", info["supply"])
	}

	// 3. mint token 1 給節點金鑰帳戶（admin，代簽者即 owner，transferFrom 的 caller==from）。
	m := studioPostJSON(t, srv, "/contract/nft/mint", map[string]any{
		"contract": contractAddr, "to": admin, "token_id": "1",
	})
	if m["ok"] != true {
		t.Fatalf("mint 失敗: %v", m)
	}
	waitFor(t, func() bool {
		q := studioGetJSON(t, srv, "/contract/nft/"+contractAddr+"?holder="+admin+"&token_id=1")
		return q["balance"] == "1" && q["owner"] != ""
	}, 8*time.Second, "mint 未上鏈")

	q := studioGetJSON(t, srv, "/contract/nft/"+contractAddr+"?holder="+admin+"&token_id=1")
	if q["balance"] != "1" {
		t.Fatalf("admin balance 應為 1，得到 %v", q["balance"])
	}
	admin0x, _ := tx0To0x(admin)
	adminKey, _ := holderKey(admin0x)
	ownerWant := vm.IntToBytes(adminKey, 32)
	if q["owner"] != new(big.Int).SetBytes(ownerWant).String() {
		t.Fatalf("ownerOf(1) 應為 admin，得到 %v", q["owner"])
	}
	if q["supply"] != "1" {
		t.Fatalf("supply 應為 1，得到 %v", q["supply"])
	}

	// 4. transferFrom admin → bob。
	tf := studioPostJSON(t, srv, "/contract/nft/transfer", map[string]any{
		"contract": contractAddr, "from": admin, "to": bob, "token_id": "1",
	})
	if tf["ok"] != true {
		t.Fatalf("transfer 失敗: %v", tf)
	}
	waitFor(t, func() bool {
		qq := studioGetJSON(t, srv, "/contract/nft/"+contractAddr+"?holder="+admin+"&token_id=1")
		return qq["balance"] == "0"
	}, 8*time.Second, "transfer 未上鏈")
	q2 := studioGetJSON(t, srv, "/contract/nft/"+contractAddr+"?holder="+bob+"&token_id=1")
	if q2["balance"] != "1" {
		t.Fatalf("bob balance 應為 1，得到 %v", q2["balance"])
	}
	bob0x, _ := tx0To0x(bob)
	bobKey, _ := holderKey(bob0x)
	if q2["owner"] != new(big.Int).SetBytes(vm.IntToBytes(bobKey, 32)).String() {
		t.Fatalf("ownerOf(1) 應為 bob，得到 %v", q2["owner"])
	}

	// 5. 重複 mint 同一 tokenId 應 REVERT（只讀模擬呼叫）。
	call := studioGetJSON(t, srv, "/contract/call/"+contractAddr+"?calldata="+vmHex(vm.Erc721MintCalldata(admin0x, big.NewInt(1))))
	if call["reverted"] != true {
		t.Fatalf("重複 mint 應 REVERT，得到 %v", call)
	}
	if strings.Contains(call["error"].(string), "invalid") {
		t.Fatalf("重複 mint 應為 REVERT 而非執行錯誤: %v", call["error"])
	}
}
