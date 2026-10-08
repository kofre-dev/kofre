"""Homologação reproduzível; somente fixtures locais, sem credenciais reais."""
import argparse
import datetime
import hashlib
import json
import pathlib
import subprocess
import sys
import time

CLIENTE_OBRIGATORIOS = [
    "TestAssinaturaIsolaAutorEDestinatario",
    "TestPainelSenhaNaoApareceELimpaAoCancelarEConfirmar",
    "TestPainelEsperaCancelaContextoAntesDeVoltar",
    "TestCancelarConviteOuRenovacaoNaoEnviaAlteracao",
    "TestRunnerPersisteBloqueioEntreChamadas",
    "TestExclusaoExigeConfirmacaoNaListaENoDetalhe",
    "TestCorporativoCadastroOuEntradaRetomaCompra",
    "TestCorporativoNaoCompraDepoisDeCadastroCanceladoOuFalha",
    "TestAtivarNuvemContaExistenteNaoRepeteCadastro",
    "TestSincronizacaoContaExigeSelecaoExplicita",
    "TestPainelExplicacaoAcompanhaSelecaoSemExecutarAcao",
    "TestPainelContaInicialTemCincoOpcoesExplicadas",
    "TestFormularioCategoriaPrimeiroESegredoVazioComNotasLegiveis",
    "TestIndicadorNuvemDistingueContaDeSincronizacao",
    "TestTelegramSomenteQuandoProConfigurado",
    "TestTelegramFalhaPermaneceVisivelSemFecharCofre",
    "TestCorporativoVoltarDosGruposNaoExecutaAcao",
    "TestCorporativoConviteRecebidoVoltarNaoAceita",
    "TestCorporativoSemEmpresasNaoEErro",
]
CLOUD_OBRIGATORIOS = [
    "TestCorporativoIsolamentoEPersistencia",
    "TestCorporativoAssentosConcorrentes",
    "TestCorporativoClienteReal",
    "TestCorporativoAdministradorDelegadoNaoAlteraContrato",
    "TestCorporativoRejeitaPapeisCorrompidos",
    "TestBancoCorporativoPreservaIsolamentoEAssentos",
    "TestEmailBackupAssinadoCASIsolamentoESessaoExpirada",
    "TestRevisaoEquipeSemCopiaNaoEditaNemExcluiSegredo",
    "TestCorporativoPagoAtivaAssentosSemProPessoalEEstornoFechaAcesso",
    "TestCorporativoSandboxNaoCriaOrganizacaoReal",
    "TestOfertaV2FreeSincronizaSemGanharPro",
    "TestCorporativoConviteEmailAntesDoCadastroIsolaDestinatario",
    "TestCorporativoRecusarConviteImpedeEntrada",
    "TestCorporativoSessaoEmailConvidaEAceitaSemCadastroPrevio",
]


def procedencia(repo):
    # Imagens não incluem .git. O hash das fontes permite identificar também
    # esse build, sem exigir metadados administrativos dentro do container.
    digest = hashlib.sha256()
    fontes = sorted(p for p in repo.rglob("*") if p.is_file()
                    and not {".git", "build", "dist", "tmp", "__pycache__"}.intersection(p.relative_to(repo).parts)
                    and (p.suffix in {".go", ".py", ".sql", ".yaml", ".yml"}
                         or p.name in {"go.mod", "go.sum", "Dockerfile"}))
    for fonte in fontes:
        digest.update(fonte.relative_to(repo).as_posix().encode())
        digest.update(b"\0")
        digest.update(fonte.read_bytes())
        digest.update(b"\0")
    try:
        sha = subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=repo, text=True,
                                      stderr=subprocess.DEVNULL).strip()
        alterado = bool(subprocess.check_output(["git", "status", "--porcelain"], cwd=repo,
                                                text=True, stderr=subprocess.DEVNULL).strip())
    except (subprocess.CalledProcessError, FileNotFoundError):
        sha, alterado = None, None
    return {"commit": sha, "alteracoes_locais": alterado, "fontes_sha256": digest.hexdigest()}


def executar(repo, comando, arquivo, obrigatorios=(), repeticoes=1):
    inicio = time.monotonic()
    print(f"[{repo.name}] {' '.join(comando)}", flush=True)
    with arquivo.open("w", encoding="utf-8") as log:
        resultado = subprocess.run(comando, cwd=repo, stdout=log, stderr=subprocess.STDOUT,
                                   text=True, encoding="utf-8", timeout=900)
    if resultado.returncode:
        raise RuntimeError(f"Gate reprovado ({resultado.returncode}). Consulte {arquivo}")
    eventos = []
    if "-json" in comando:
        for linha in arquivo.read_text(encoding="utf-8").splitlines():
            try:
                eventos.append(json.loads(linha))
            except json.JSONDecodeError:
                # Diagnósticos do compilador não são eventos. O exit code e
                # a presença dos testes obrigatórios continuam obrigatórios.
                pass
        if any(e.get("Action") == "fail" for e in eventos):
            raise RuntimeError(f"Evento de falha em {arquivo}")
        for nome in obrigatorios:
            aprovados = sum(e.get("Action") == "pass" and e.get("Test") == nome for e in eventos)
            pulados = any(e.get("Action") == "skip" and e.get("Test", "").split("/")[0] == nome
                          for e in eventos)
            if aprovados < repeticoes or pulados:
                raise RuntimeError(f"Teste obrigatório ausente, pulado ou incompleto: {nome}")
    return {"comando": comando, "segundos": round(time.monotonic()-inicio, 2),
            "testes_aprovados": sum(e.get("Action") == "pass" and bool(e.get("Test")) for e in eventos),
            "log": arquivo.name}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--cloud", type=pathlib.Path, help="Checkout irmão kofre-cloud para homologação completa")
    parser.add_argument("--repeticoes", type=int, default=5)
    parser.add_argument("--saida", type=pathlib.Path)
    args = parser.parse_args()
    if not 1 <= args.repeticoes <= 100:
        parser.error("repeticoes deve ficar entre 1 e 100")
    cliente = pathlib.Path(__file__).resolve().parents[1]
    cloud = args.cloud.resolve() if args.cloud else None
    saida = (args.saida or cliente / "build" / "homologacao").resolve()
    saida.mkdir(parents=True, exist_ok=True)
    relatorio = {"inicio": datetime.datetime.now(datetime.timezone.utc).isoformat(),
                 "escopo": "cliente-e-cloud" if cloud else "somente-cliente",
                 "repeticoes": args.repeticoes, "aprovado": False, "gates": []}
    try:
        if cloud and (not (cloud / "pkg/server").is_dir() or cloud.parent / "kofre" != cliente):
            raise RuntimeError("A integração exige checkouts irmãos chamados kofre e kofre-cloud")
        repos = [("cliente", cliente)] + ([("cloud", cloud)] if cloud else [])
        for nome, repo in repos:
            relatorio[nome + "_procedencia"] = procedencia(repo)
            relatorio["gates"].append(executar(repo, ["go", "vet", "./..."], saida / (nome + "-vet.log")))
            obrigatorios = CLIENTE_OBRIGATORIOS if nome == "cliente" else CLOUD_OBRIGATORIOS
            relatorio["gates"].append(executar(repo, ["go", "test", "-race", "./...", "-count=1", "-json"],
                                                saida / (nome + "-suite.jsonl"), obrigatorios))
            if nome == "cliente":
                pacotes = ["./pkg/corporativo", "./pkg/conta", "./pkg/runner", "./pkg/tui", "./cmd/kofre"]
                selecao = "Test(Assinatura|Painel|Cancelar|Runner|Email|EntradaVisual|ResumoAssinatura|Exclusao|Corporativo|AtivarNuvem|SincronizacaoConta|Formulario|IndicadorNuvem|Telegram)"
            else:
                pacotes = ["./pkg/server"]
                selecao = "Test(Corporativo|BancoCorporativo|Email|Revisao|OfertaV2|RenovacaoCorporativa)"
            relatorio["gates"].append(executar(repo,
                ["go", "test", "-race", *pacotes, "-run", selecao, f"-count={args.repeticoes}", "-json"],
                saida / (nome + "-regras.jsonl"), obrigatorios, args.repeticoes))
        relatorio["aprovado"] = True
        print("Homologação aprovada: " + relatorio["escopo"], flush=True)
    except (RuntimeError, subprocess.SubprocessError, OSError) as erro:
        relatorio["erro"] = str(erro)
        print("Homologação REPROVADA: " + str(erro), file=sys.stderr, flush=True)
    finally:
        relatorio["fim"] = datetime.datetime.now(datetime.timezone.utc).isoformat()
        (saida / "relatorio.json").write_text(json.dumps(relatorio, ensure_ascii=False, indent=2), encoding="utf-8")
    return 0 if relatorio["aprovado"] else 1


if __name__ == "__main__":
    sys.exit(main())
