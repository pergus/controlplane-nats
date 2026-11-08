#VirtualMachine: {
	kind: "VirtualMachine"
	generation: int & >=1
	metadata: {
		name: string & =~"^[A-Za-z0-9_-]+$"
	}
	spec: {
		cpu: int & >=1 & <=128
		memory: int & >=512
		image: string
		networks?: [...string]
	}
}
