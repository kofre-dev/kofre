"""Verifica o próprio gate para não aceitar testes ausentes ou pulados."""
import json
import pathlib
import sys
import tempfile
import unittest

from homologar import executar


class GateHomologacao(unittest.TestCase):
    def rodar(self, eventos, repeticoes=1):
        with tempfile.TemporaryDirectory() as pasta:
            repo = pathlib.Path(pasta)
            linhas = "\n".join(json.dumps(e) for e in eventos)
            return executar(repo, [sys.executable, "-c", f"print({linhas!r})", "-json"],
                            repo / "gate.jsonl", ["TestFronteira"], repeticoes)

    def test_reprova_exit_zero_sem_teste(self):
        with self.assertRaises(RuntimeError):
            self.rodar([])

    def test_reprova_teste_pulado(self):
        with self.assertRaises(RuntimeError):
            self.rodar([{"Action": "skip", "Test": "TestFronteira"}])

    def test_reprova_repeticoes_incompletas(self):
        with self.assertRaises(RuntimeError):
            self.rodar([{"Action": "pass", "Test": "TestFronteira"}], 2)

    def test_reprova_evento_falha(self):
        with self.assertRaises(RuntimeError):
            self.rodar([{"Action": "pass", "Test": "TestFronteira"}, {"Action": "fail"}])

    def test_aprova_teste_realmente_repetido(self):
        evento = {"Action": "pass", "Test": "TestFronteira"}
        self.assertEqual(self.rodar([evento, evento], 2)["testes_aprovados"], 2)


if __name__ == "__main__":
    unittest.main()
