pragma solidity ^0.8.2;

contract Store {
	function increment(uint input) public pure returns (uint)  {
		if (input == type(uint).max) {
			return 0;
		}
		return input + 1;
	}
}
