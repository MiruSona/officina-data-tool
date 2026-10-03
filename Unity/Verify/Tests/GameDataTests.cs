using System;
using System.IO;
using System.Linq;
using System.Reflection;
using System.Text;
using MessagePack;
using MyGame.Data;
using NUnit.Framework;
using UnityEngine;

namespace DataToolVerify.Tests
{
    // verify.ps1 이 Unity 프로젝트에 복사해 넣고 배치모드로 돌리는 EditMode 시험이다 (U1~U4 · U4b).
    // 입력은 DataTool/Testdata/table/ok 를 gen · export 한 것이다. 기대값은 Testdata/roundtrip-cs 와 같다.
    public class GameDataTests
    {
        private const string U4bPath = "Tests/DataToolVerify/gamedata-u4b.bytes";

        private static byte[] ReadBaked()
        {
            string path = Path.Combine(Application.streamingAssetsPath, GameDataLoader.FileName);
            Assert.IsTrue(File.Exists(path), "구운 파일이 없다 : " + path);
            return File.ReadAllBytes(path);
        }

        // U1 : 소스 제너레이터가 생성 어셈블리 안에 리졸버와 포매터를 만들었나.
        [Test]
        public void U1_SourceGeneratorMadeFormatters()
        {
            Type[] types = typeof(ItemRow).Assembly.GetTypes();
            Type resolver = types.FirstOrDefault(t => t.Name == "GeneratedMessagePackResolver");
            Assert.IsNotNull(resolver, "GeneratedMessagePackResolver 가 없다 — 소스 제너레이터가 안 돌았다");

            string[] formatters = types.Where(t => t.Name.EndsWith("Formatter")).Select(t => t.FullName).ToArray();
            Debug.Log("DATATOOL-VERIFY formatters : " + string.Join(", ", formatters));
            Assert.IsTrue(formatters.Any(n => n.Contains("ItemRowFormatter")), "ItemRowFormatter 가 없다");
            Assert.IsTrue(formatters.Any(n => n.Contains("MonsterRowFormatter")), "MonsterRowFormatter 가 없다");
            Assert.IsTrue(formatters.Any(n => n.Contains("DropRowFormatter")), "DropRowFormatter 가 없다");

            FieldInfo field = resolver.GetField("Instance", BindingFlags.Public | BindingFlags.Static);
            Assert.IsNotNull(field, "생성 리졸버에 Instance 가 없다");
            IFormatterResolver instance = (IFormatterResolver)field.GetValue(null);
            Assert.IsNotNull(instance.GetFormatter<ItemRow>(), "생성 리졸버가 ItemRow 포매터를 못 준다");
        }

        // U2·U3 : Go 가 구운 것을 C# 이 읽고, 한글·float·enum·기본값·asset·ref 가 맞나.
        [Test]
        public void U2_U3_LoadBakedAndCompareValues()
        {
            GameDataTables t = GameDataLoader.Load(ReadBaked());

            Assert.AreEqual("1db5aa5f0ede6b0a", GameDataTables.SchemaHash);
            Assert.AreEqual(6, t.ItemRows.Count);
            Assert.AreEqual(4, t.MonsterRows.Count);
            Assert.AreEqual(8, t.DropRows.Count);

            ItemRow first = t.ItemRows[0];
            Assert.AreEqual("sword_iron", first.Id);
            Assert.AreEqual("철검", first.Name);
            Assert.AreEqual(12, first.Atk);
            Assert.AreEqual(300f, first.Price);
            Assert.AreEqual(Grade.Common, first.Grade);
            CollectionAssert.AreEqual(new[] { "weapon", "melee" }, first.Tags);

            // 빠진 열은 기본값이다 — atk 0 · tags 빈 배열 · counts [1].
            ItemRow potion = t.Item["potion_hp"];
            Assert.AreEqual("체력 물약", potion.Name);
            Assert.AreEqual(0, potion.Atk);
            Assert.IsTrue(potion.Usable);
            Assert.AreEqual(Grade.Epic, t.Item["gem_fire"].Grade);
            Assert.AreEqual(0, t.Item["gem_fire"].Tags.Length);

            Assert.AreEqual("초록 슬라임", t.MonsterRows[0].Name);
            Assert.AreEqual(Element.Ice, t.Monster["slime_blue"].Element);
            Assert.AreEqual("icons[icon_sword]", t.MonsterRows[0].Icon);

            Assert.AreEqual(0.05f, t.Drop["drop_wolf_sword"].Rate);
            Assert.AreEqual(0.01f, t.Drop["drop_slime_g_gem"].Rate);
            CollectionAssert.AreEqual(new[] { 1 }, t.DropRows[0].Counts);
            Assert.AreEqual(3, t.Drop["drop_golem_gem"].Counts[2]);

            ItemRow dropped = t.Get<ItemRow>(t.DropRows[0].ItemId);
            Assert.AreEqual("potion_hp", dropped.Id);
        }

        // U4 : 해시 16글자만 0 으로 덮은 파일은 GameDataException 이다 (길이가 같아 자리가 안 밀린다).
        [Test]
        public void U4_SchemaHashMismatchThrows()
        {
            byte[] raw = ReadBaked();
            OverwriteAscii(raw, GameDataTables.SchemaHash, "0000000000000000");

            GameDataException e = Assert.Throws<GameDataException>(() => GameDataLoader.Load(raw));
            StringAssert.Contains("스키마가 달라졌는데", e.Message);
            Debug.Log("DATATOOL-VERIFY U4 message : " + e.Message);
        }

        // U4b : 열 타입을 바꾼(item.atk int → string) 스키마로 구운 옛 파일.
        // ① 그대로면 _meta 자리의 해시 검사로 GameDataException — MessagePack 예외가 먼저 나면 실패다.
        // ② 해시를 코드 것으로 속여도 표 읽기 예외가 GameDataException 으로 싸여 나온다.
        [Test]
        public void U4b_OldBakeAfterColumnTypeChangeThrows()
        {
            string path = Path.Combine(Application.dataPath, U4bPath);
            Assert.IsTrue(File.Exists(path), "U4b 자료가 없다 : " + path);
            byte[] raw = File.ReadAllBytes(path);

            GameDataException hashError = Assert.Throws<GameDataException>(() => GameDataLoader.Load(raw));
            StringAssert.Contains("스키마가 달라졌는데", hashError.Message);
            Debug.Log("DATATOOL-VERIFY U4b hash message : " + hashError.Message);

            string bakedHash = FindMetaHash(raw);
            OverwriteAscii(raw, bakedHash, GameDataTables.SchemaHash);
            GameDataException tableError = Assert.Throws<GameDataException>(() => GameDataLoader.Load(raw));
            Assert.IsInstanceOf<MessagePackSerializationException>(tableError.InnerException,
                "표 읽기 예외가 MessagePack 예외를 안고 있어야 한다");
            Debug.Log("DATATOOL-VERIFY U4b table message : " + tableError.Message);
        }

        // 구운 파일의 _meta.schemaHash 값(16글자)을 바이트에서 찾는다. 키 바로 뒤가 fixstr(0xb0) 머리다.
        private static string FindMetaHash(byte[] raw)
        {
            byte[] key = Encoding.ASCII.GetBytes("schemaHash");
            int at = IndexOf(raw, key);
            Assert.GreaterOrEqual(at, 0, "구운 파일에서 schemaHash 키를 못 찾았다");
            int valueAt = at + key.Length;
            Assert.AreEqual(0xb0, raw[valueAt], "schemaHash 값이 16글자 fixstr 이 아니다");
            return Encoding.ASCII.GetString(raw, valueAt + 1, 16);
        }

        private static void OverwriteAscii(byte[] raw, string from, string to)
        {
            Assert.AreEqual(from.Length, to.Length, "덮어쓸 글자 수가 다르다");
            byte[] needle = Encoding.ASCII.GetBytes(from);
            int at = IndexOf(raw, needle);
            Assert.GreaterOrEqual(at, 0, "구운 파일 안에서 " + from + " 을 못 찾았다");
            byte[] patch = Encoding.ASCII.GetBytes(to);
            Array.Copy(patch, 0, raw, at, patch.Length);
        }

        private static int IndexOf(byte[] hay, byte[] needle)
        {
            for (int i = 0; i + needle.Length <= hay.Length; i++)
            {
                int j = 0;
                while (j < needle.Length && hay[i + j] == needle[j])
                {
                    j++;
                }

                if (j == needle.Length)
                {
                    return i;
                }
            }

            return -1;
        }
    }
}
