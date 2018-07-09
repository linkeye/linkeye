package params

const (
	// These are the multipliers for ether denominations.
	// Example: To get the wei value of an amount in 'GuoSanBin', use
	//
	//    new(big.Int).Mul(value, big.NewInt(params.GuoSanBin))
	//
	Lei        = 1
	DingWei    = 1e3
	WangTieSuo = 1e6
	GuoSanBin  = 1e9
	YuXiaoWei  = 1e12
	LuZeng     = 1e15
	Leter      = 1e18
	HuangXue   = 1e21
	LiYiBo     = 1e42
)
