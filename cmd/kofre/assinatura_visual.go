package main

import (
	"fmt"
	"strings"
	"time"
)

// Apenas apresenta os dados devolvidos pelo Cloud, sem calcular regras do plano.
func resumoAssinatura(contrato map[string]any) string {
	validade := fmt.Sprint(contrato["validade"])
	if data, err := time.Parse(time.RFC3339, validade); err == nil {
		validade = data.Local().Format("02/01/2006")
	}
	renovacao := "Manual"
	if contrato["recorrente"] == true {
		renovacao = "Automática"
	}
	if contrato["cancelado"] == true {
		renovacao = "Cancelada · acesso até o fim do período pago"
	}
	var texto strings.Builder
	fmt.Fprintf(&texto, "Assentos contratados: %v\nAssentos ocupados: %v\nValidade: %s\nRenovação: %s\n\nPedido: %v", contrato["assentos"], contrato["ocupados"], validade, renovacao, contrato["pedido_id"])
	if ajustes, ok := contrato["ajustes"].([]any); ok && len(ajustes) > 0 {
		texto.WriteString("\n\nAjustes de assentos:")
		for _, valor := range ajustes {
			if ajuste, ok := valor.(map[string]any); ok {
				fmt.Fprintf(&texto, "\n  %v assentos · %v", ajuste["quantidade"], ajuste["status"])
			}
		}
	}
	if ciclos, ok := contrato["ciclos"].([]any); ok {
		for _, valor := range ciclos {
			if ciclo, ok := valor.(map[string]any); ok && ciclo["confirmado"] != true {
				fmt.Fprintf(&texto, "\n\nPróximo ciclo: %v assentos · início %v", ciclo["quantidade"], ciclo["inicio"])
			}
		}
	}
	return texto.String()
}
