using System;
using System.IO;
using UnityEditor;
using UnityEditor.Build;
using UnityEditor.Build.Reporting;
using UnityEditor.SceneManagement;
using UnityEngine;

// verify.ps1 의 U5 빌드 진입점이다.
//   Unity.exe -batchmode -nographics -projectPath <P> -executeMethod DataToolVerifyBuild.BuildWin64Il2Cpp -buildOut <폴더> [-stripping High]
// 빈 장면 하나로 Windows 64 IL2CPP 플레이어(Verify.exe)를 굽는다.
// 종료 코드 : 0 빌드 성공 · 1 빌드 실패(코드를 고칠 일) · 2 환경 문제(인자가 틀렸거나 예외 — 기계·호출을 고칠 일).
// 빌드 동안 Standalone scriptingBackend 를 IL2CPP 로, -stripping 을 주면 스트리핑도 바꾸지만
// 끝나면(성공·실패·예외 모두) **바꾸기 전 값으로 되돌리고 ProjectSettings 를 저장한다**. 구운 플레이어는 그대로 둔다.
public static class DataToolVerifyBuild
{
    private const string ScenePath = "Assets/DataToolVerify/Verify.unity";
    private const string PlayerName = "Verify.exe";
    private const int ExitOk = 0;
    private const int ExitBuildFailed = 1;
    private const int ExitEnv = 2;

    public static void BuildWin64Il2Cpp()
    {
        int code = ExitEnv;
        try
        {
            code = Build();
        }
        catch (Exception e)
        {
            Debug.LogError("DATATOOL-VERIFY 환경 문제 — 빌드 중 예외 " + e);
            code = ExitEnv;
        }

        EditorApplication.Exit(code);
    }

    private static int Build()
    {
        string outDir = ArgAfter("-buildOut");
        if (string.IsNullOrEmpty(outDir))
        {
            Debug.LogError("DATATOOL-VERIFY 환경 문제 — -buildOut <폴더> 가 없다");
            return ExitEnv;
        }

        ManagedStrippingLevel? level = null;
        string strip = ArgAfter("-stripping");
        if (strip != null)
        {
            ManagedStrippingLevel parsed;
            if (Enum.TryParse(strip, false, out parsed) == false || Enum.IsDefined(typeof(ManagedStrippingLevel), parsed) == false)
            {
                Debug.LogError("DATATOOL-VERIFY 환경 문제 — -stripping 값을 모른다 : " + strip);
                return ExitEnv;
            }

            level = parsed;
        }

        EnsureScene();

        // 바꾸기 전 값을 기억해 두고 finally 에서 되돌린다. 사용자 프로젝트 설정을 검증이 바꿔 놓지 않게.
        ScriptingImplementation oldBackend = PlayerSettings.GetScriptingBackend(NamedBuildTarget.Standalone);
        ManagedStrippingLevel oldStripping = PlayerSettings.GetManagedStrippingLevel(NamedBuildTarget.Standalone);
        try
        {
            PlayerSettings.SetScriptingBackend(NamedBuildTarget.Standalone, ScriptingImplementation.IL2CPP);
            if (level.HasValue)
            {
                PlayerSettings.SetManagedStrippingLevel(NamedBuildTarget.Standalone, level.Value);
            }

            Debug.Log("DATATOOL-VERIFY backend=" + PlayerSettings.GetScriptingBackend(NamedBuildTarget.Standalone) +
                " stripping=" + PlayerSettings.GetManagedStrippingLevel(NamedBuildTarget.Standalone));

            BuildPlayerOptions options = new BuildPlayerOptions
            {
                scenes = new[] { ScenePath },
                locationPathName = Path.Combine(outDir, PlayerName),
                target = BuildTarget.StandaloneWindows64,
                options = BuildOptions.None,
            };

            BuildReport report = BuildPipeline.BuildPlayer(options);
            BuildSummary s = report.summary;
            Debug.Log("DATATOOL-VERIFY build result=" + s.result + " errors=" + s.totalErrors + " warnings=" + s.totalWarnings +
                " time=" + s.totalTime + " size=" + s.totalSize);
            if (s.result == BuildResult.Succeeded)
            {
                return ExitOk;
            }

            return ExitBuildFailed;
        }
        finally
        {
            PlayerSettings.SetScriptingBackend(NamedBuildTarget.Standalone, oldBackend);
            // 스트리핑은 바꿨을 때만 되돌린다. 설정 줄이 없으면 getter 는 백엔드별 기본값(Mono 면 Disabled)을 돌려주는데,
            // 그 값을 명시로 써 넣으면 나중에 IL2CPP 로 바꿨을 때 기본(Minimal) 대신 Disabled 가 되어 버린다.
            if (level.HasValue)
            {
                PlayerSettings.SetManagedStrippingLevel(NamedBuildTarget.Standalone, oldStripping);
            }

            AssetDatabase.SaveAssets();
            Debug.Log("DATATOOL-VERIFY restored backend=" + oldBackend + (level.HasValue ? " stripping=" + oldStripping : ""));
        }
    }

    // Build Settings 장면 목록은 건드리지 않는다. 빌드에 넘길 빈 장면만 없으면 만든다.
    private static void EnsureScene()
    {
        if (File.Exists(ScenePath))
        {
            return;
        }

        Directory.CreateDirectory(Path.GetDirectoryName(ScenePath));
        var scene = EditorSceneManager.NewScene(NewSceneSetup.EmptyScene, NewSceneMode.Single);
        EditorSceneManager.SaveScene(scene, ScenePath);
    }

    private static string ArgAfter(string name)
    {
        string[] args = Environment.GetCommandLineArgs();
        for (int i = 0; i < args.Length - 1; i++)
        {
            if (args[i] == name)
            {
                return args[i + 1];
            }
        }

        return null;
    }
}
