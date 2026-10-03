using System;
using MyGame.Data;
using UnityEngine;

// verify.ps1 의 U5 플레이어 탐침이다. IL2CPP 플레이어가 뜨자마자 구운 파일을 읽고 결과를 로그에 찍은 뒤 끝난다.
// -datatoolVerify 인자가 있을 때만 돈다 — 이 프로젝트의 다른 빌드(게임 빌드)에 섞여도 아무 일 안 한다.
// 종료 코드 0 = 통과, 1 = 값 어긋남, 2 = 예외.
public static class DataToolVerifyRunner
{
    private const string Flag = "-datatoolVerify";

    [RuntimeInitializeOnLoadMethod(RuntimeInitializeLoadType.AfterSceneLoad)]
    private static void Run()
    {
        if (Array.IndexOf(Environment.GetCommandLineArgs(), Flag) < 0)
        {
            return;
        }

        int code;
        try
        {
            GameDataTables t = GameDataLoader.LoadFromStreamingAssets();
            int bad = 0;
            bad += Same("item rows", t.ItemRows.Count, 6);
            bad += Same("name", t.ItemRows[0].Name, "철검");
            bad += Same("grade", t.Item["gem_fire"].Grade, Grade.Epic);
            bad += Same("rate", t.Drop["drop_wolf_sword"].Rate, 0.05f);
            bad += Same("counts", t.Drop["drop_golem_gem"].Counts[2], 3);
            bad += Same("ref", t.Get<ItemRow>(t.DropRows[0].ItemId).Id, "potion_hp");
            Debug.Log("PROBE platform=" + Application.platform + " il2cpp=" + IsIl2Cpp() + " resolver=" +
                MessagePack.MessagePackSerializer.DefaultOptions.Resolver.GetType().FullName);
            if (bad == 0)
            {
                Debug.Log("PROBE RESULT OK");
                code = 0;
            }
            else
            {
                Debug.Log("PROBE RESULT MISMATCH " + bad);
                code = 1;
            }
        }
        catch (Exception e)
        {
            Debug.Log("PROBE RESULT EXCEPTION " + e);
            code = 2;
        }

        Application.Quit(code);
    }

    private static int Same<T>(string what, T got, T want)
    {
        bool ok = Equals(got, want);
        string mark = "BAD ";
        if (ok)
        {
            mark = "ok  ";
        }

        Debug.Log("PROBE " + mark + what + " got=" + got + " want=" + want);
        if (ok)
        {
            return 0;
        }

        return 1;
    }

    private static bool IsIl2Cpp()
    {
#if ENABLE_IL2CPP
        return true;
#else
        return false;
#endif
    }
}
