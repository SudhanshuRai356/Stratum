package dht

import (
	"encoding/json"
	"fmt"
	"os"
)
func SaveNodes(filepath string,node []Node)error{ //saveing nodes to json
	valid_nodes:=[]Node{}
	for _,n:=range node{
		if n.Validate()==nil{
			valid_nodes=append(valid_nodes,n)
		}else{
			fmt.Printf("Invalid node skipped: %v\n", n)
		}
	}
	valid_nodes_json, err := json.MarshalIndent(valid_nodes, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal valid nodes: %w", err)
	}
	err = os.WriteFile(filepath, valid_nodes_json, 0644)
	if err != nil {
		return fmt.Errorf("failed to write nodes to file: %w", err)
	}
	fmt.Printf("Nodes saved to %s\n", filepath)
	return nil
}
func LoadNodes(filepath string) []Node{ //loading saved nodes
	nodes,err:=os.ReadFile(filepath)
	if err!=nil{
		if os.IsNotExist(err) {
			fmt.Printf("No such node yet, error: %v\n", err)
			return []Node{}
		}
		fmt.Printf("Failed to read nodes from file: %v\n", err)
		return []Node{}
	}
	var loaded_Nodes []Node
	err = json.Unmarshal(nodes, &loaded_Nodes)
	if err != nil {
		fmt.Printf("Failed to unmarshal nodes: %v\n", err)
		return []Node{}
	}
	true_nodes:=[]Node{}
	for _,node:=range loaded_Nodes{
		if node.Validate()==nil{
			true_nodes=append(true_nodes,node)
		}else{
			fmt.Printf("Invalid node skipped: %v\n", node)
		}
	}
	return true_nodes
}