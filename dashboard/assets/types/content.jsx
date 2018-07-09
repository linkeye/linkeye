// @flow


export type Content = {
	general: General,
	home: Home,
	chain: Chain,
	txpool: TxPool,
	network: Network,
	system: System,
	logs: Logs,
};

export type ChartEntries = Array<ChartEntry>;

export type ChartEntry = {
	time: Date,
	value: number,
};

export type General = {
    version: ?string,
    commit: ?string,
};

export type Home = {
	/* TODO (kurkomisi) */
};

export type Chain = {
	/* TODO (kurkomisi) */
};

export type TxPool = {
	/* TODO (kurkomisi) */
};

export type Network = {
	/* TODO (kurkomisi) */
};

export type System = {
    activeMemory: ChartEntries,
    virtualMemory: ChartEntries,
    networkIngress: ChartEntries,
    networkEgress: ChartEntries,
    processCPU: ChartEntries,
    systemCPU: ChartEntries,
    diskRead: ChartEntries,
    diskWrite: ChartEntries,
};

export type Logs = {
	log: Array<string>,
};
