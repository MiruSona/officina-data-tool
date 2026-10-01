using System;
using System.Collections.Generic;
using System.Globalization;
using System.IO;
using MyGame.Data;
// 로더(GameDataLoader·GameDataException)도 gen 이 MyGame.Data 로 같이 내준다.

namespace DataTool.Roundtrip
{
    // Go 가 구운 gamedata.bytes 를 진짜 MessagePack-CSharp 로 읽히는지 Unity 없이 본다.
    // 쓰는 법 : Roundtrip <gamedata.bytes 경로> <기대 schemaHash> [<해시 어긋난 파일>]
    // 하나라도 어긋나면 종료 1 이다.
    public static class Program
    {
        private static int failed;
        private static int checkedCount;

        public static int Main(string[] args)
        {
            if (args.Length < 2)
            {
                Console.Error.WriteLine("쓰는 법 : Roundtrip <gamedata.bytes> <schemaHash> [<해시 어긋난 파일>]");
                return 2;
            }

            string path = args[0];
            string wantHash = args[1];
            string badPath = args.Length > 2 ? args[2] : null;

            Console.WriteLine("MessagePack 판 : " + typeof(MessagePack.MessagePackSerializer).Assembly.GetName().Version);
            Console.WriteLine(".NET 판 : " + Environment.Version);
            Console.WriteLine("읽을 파일 : " + path);
            Console.WriteLine();

            GameDataTables t = GameDataLoader.LoadFromFile(path);

            Console.WriteLine("== 머리·메타 ==");
            Console.WriteLine("  builtAt : " + t.BuiltAt);
            Console.WriteLine("  코드 schemaHash : " + GameDataTables.SchemaHash);
            Check("schemaHash", GameDataTables.SchemaHash, wantHash);
            Check("builtAt 가 비지 않았다", string.IsNullOrEmpty(t.BuiltAt) == false, true);

            CheckItem(t);
            CheckMonster(t);
            CheckDrop(t);
            CheckRef(t);
            CheckBadHash(badPath);

            Console.WriteLine();
            Console.WriteLine("확인 " + checkedCount + "건 · 어긋남 " + failed + "건");
            return failed == 0 ? 0 : 1;
        }

        private static void CheckItem(GameDataTables t)
        {
            Console.WriteLine();
            Console.WriteLine("== 표 item ==");
            Check("item 행 수", t.ItemRows.Count, 6);

            ItemRow first = t.ItemRows[0];
            Dump("  첫 행", "id=" + first.Id + " name=" + first.Name + " atk=" + first.Atk +
                " price=" + F(first.Price) + " usable=" + first.Usable +
                " gradeName=" + first.GradeName + " grade=" + first.Grade +
                " tags=[" + string.Join(",", first.Tags) + "]");

            Check("item[0].Id", first.Id, "sword_iron");
            Check("item[0].Name (한글)", first.Name, "철검");
            Check("item[0].Atk (int)", first.Atk, 12);
            Check("item[0].Price (float)", first.Price, 300f);
            Check("item[0].Usable (bool 기본값 false)", first.Usable, false);
            Check("item[0].GradeName (enum 이름 기본값)", first.GradeName, "common");
            Check("item[0].Grade (enum 값)", first.Grade, Grade.Common);
            Check("item[0].Tags 길이 (list<string>)", first.Tags.Length, 2);
            Check("item[0].Tags[0]", first.Tags[0], "weapon");
            Check("item[0].Tags[1]", first.Tags[1], "melee");

            ItemRow potion = t.Item["potion_hp"];
            Dump("  potion_hp", "name=" + potion.Name + " atk=" + potion.Atk + " price=" + F(potion.Price) +
                " usable=" + potion.Usable + " tags=[" + string.Join(",", potion.Tags) + "]");
            Check("potion_hp.Name (한글 공백)", potion.Name, "체력 물약");
            Check("potion_hp.Atk (빠진 열이 기본값 0)", potion.Atk, 0);
            Check("potion_hp.Price", potion.Price, 50f);
            Check("potion_hp.Usable (bool true)", potion.Usable, true);

            ItemRow gem = t.Item["gem_fire"];
            Dump("  gem_fire", "name=" + gem.Name + " price=" + F(gem.Price) + " grade=" + gem.Grade +
                " tags 길이=" + gem.Tags.Length);
            Check("gem_fire.Name", gem.Name, "불의 보석");
            Check("gem_fire.Price", gem.Price, 1200f);
            Check("gem_fire.GradeName", gem.GradeName, "epic");
            Check("gem_fire.Grade (enum 이름→값)", gem.Grade, Grade.Epic);
            Check("gem_fire.Tags (기본값 빈 배열)", gem.Tags.Length, 0);

            Check("item['sword_steel'].Grade", t.Item["sword_steel"].Grade, Grade.Rare);
            Check("item 사전 크기", t.Item.Count, 6);
        }

        private static void CheckMonster(GameDataTables t)
        {
            Console.WriteLine();
            Console.WriteLine("== 표 monster ==");
            Check("monster 행 수", t.MonsterRows.Count, 4);

            MonsterRow first = t.MonsterRows[0];
            Dump("  첫 행", "id=" + first.Id + " name=" + first.Name + " hp=" + first.Hp +
                " elementName=" + first.ElementName + " element=" + first.Element);
            Check("monster[0].Id", first.Id, "slime_green");
            Check("monster[0].Name (한글 공백)", first.Name, "초록 슬라임");
            Check("monster[0].Hp (int)", first.Hp, 30);
            Check("monster[0].ElementName (기본값)", first.ElementName, "fire");
            Check("monster[0].Element", first.Element, Element.Fire);

            MonsterRow blue = t.Monster["slime_blue"];
            Dump("  slime_blue", "name=" + blue.Name + " hp=" + blue.Hp + " element=" + blue.Element);
            Check("slime_blue.Element (적힌 enum)", blue.Element, Element.Ice);
            Check("monster[0].Icon (asset 문자열)", first.Icon, "icons[icon_sword]");
            Check("monster[0].Sfx 길이 (list<asset>)", first.Sfx.Length, 1);
            Check("monster[0].Sfx[0]", first.Sfx[0], "Sfx/hit.wav");
            Check("slime_blue.Icon (asset 기본값 빈 문자열)", blue.Icon, "");
            Check("slime_blue.Sfx (기본값 빈 배열)", blue.Sfx.Length, 0);
            Check("golem_fire.Hp", t.Monster["golem_fire"].Hp, 900);
            Check("golem_fire.Name", t.Monster["golem_fire"].Name, "불의 골렘");
        }

        private static void CheckDrop(GameDataTables t)
        {
            Console.WriteLine();
            Console.WriteLine("== 표 drop ==");
            Check("drop 행 수", t.DropRows.Count, 8);

            DropRow first = t.DropRows[0];
            Dump("  첫 행", "id=" + first.Id + " monsterId=" + first.MonsterId + " itemId=" + first.ItemId +
                " rate=" + F(first.Rate) + " counts=[" + string.Join(",", first.Counts) + "]");
            Check("drop[0].Id", first.Id, "drop_slime_g_potion");
            Check("drop[0].MonsterId (ref 열)", first.MonsterId, "slime_green");
            Check("drop[0].ItemId (ref 열)", first.ItemId, "potion_hp");
            Check("drop[0].Rate (float 0.5)", first.Rate, 0.5f);
            Check("drop[0].Counts (기본값 [1]) 길이", first.Counts.Length, 1);
            Check("drop[0].Counts[0]", first.Counts[0], 1);

            DropRow sword = t.Drop["drop_wolf_sword"];
            Dump("  drop_wolf_sword", "rate=" + F(sword.Rate));
            Check("drop_wolf_sword.Rate (float32 0.05 비트 그대로)", sword.Rate, 0.05f);
            Check("drop_slime_g_gem.Rate (0.01)", t.Drop["drop_slime_g_gem"].Rate, 0.01f);
            Check("drop_golem_steel.Rate (0.02)", t.Drop["drop_golem_steel"].Rate, 0.02f);

            DropRow gem = t.Drop["drop_golem_gem"];
            Dump("  drop_golem_gem", "rate=" + F(gem.Rate) + " counts=[" + string.Join(",", gem.Counts) + "]");
            Check("drop_golem_gem.Counts 길이 (list<int>)", gem.Counts.Length, 3);
            Check("drop_golem_gem.Counts[2]", gem.Counts[2], 3);
            Check("drop_slime_b_potion.Counts 길이", t.Drop["drop_slime_b_potion"].Counts.Length, 2);
        }

        private static void CheckRef(GameDataTables t)
        {
            Console.WriteLine();
            Console.WriteLine("== ref 잇기 ==");
            DropRow row = t.Drop["drop_golem_gem"];
            MonsterRow m = t.Get<MonsterRow>(row.MonsterId);
            ItemRow it = t.Get<ItemRow>(row.ItemId);
            Dump("  drop_golem_gem →", "monster=" + (m == null ? "없음" : m.Name) +
                " item=" + (it == null ? "없음" : it.Name));
            Check("Get<MonsterRow>(ref)", m == null ? null : m.Id, "golem_fire");
            Check("Get<ItemRow>(ref)", it == null ? null : it.Id, "gem_fire");
            Check("Get<ItemRow>(없는 id) 는 null", t.Get<ItemRow>("없는거") == null, true);
            Check("GameDataLoader.Current 가 채워졌다", GameDataLoader.Current != null, true);
            Check("GameDataLoader.Get<MonsterRow> (정적)",
                GameDataLoader.Get<MonsterRow>("wolf_gray").Name, "잿빛 늑대");
        }

        private static void CheckBadHash(string badPath)
        {
            Console.WriteLine();
            Console.WriteLine("== schemaHash 어긋난 파일 ==");
            if (badPath == null)
            {
                Console.WriteLine("  (건너뜀 — 파일 인자 없음)");
                return;
            }

            try
            {
                GameDataLoader.LoadFromFile(badPath);
                Check("해시 어긋난 파일이 GameDataException 을 낸다", "예외 없음", "GameDataException");
            }
            catch (GameDataException e)
            {
                Dump("  잡은 예외", e.Message);
                Check("해시 어긋난 파일이 GameDataException 을 낸다", "GameDataException", "GameDataException");
            }
        }

        private static string F(float v)
        {
            return v.ToString("R", CultureInfo.InvariantCulture);
        }

        private static void Dump(string label, string text)
        {
            Console.WriteLine(label + " : " + text);
        }

        private static void Check<T>(string what, T got, T want)
        {
            checkedCount++;
            bool ok = EqualityComparer<T>.Default.Equals(got, want);
            if (ok == false)
            {
                failed++;
            }

            Console.WriteLine("  [" + (ok ? "맞음" : "어긋남") + "] " + what +
                (ok ? " = " + got : " : 받은 값 " + got + " · 기대 " + want));
        }
    }
}
