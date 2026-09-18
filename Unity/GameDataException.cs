using System;

namespace Officina.Data
{
    // 구운 데이터를 읽다가 어긋난 자리에서 던지는 예외다.
    // 조용히 반쯤 읽는 것보다 바로 터지는 것이 낫다 (설계 8장).
    public sealed class GameDataException : Exception
    {
        public GameDataException(string message)
            : base(message)
        {
        }

        public GameDataException(string message, Exception inner)
            : base(message, inner)
        {
        }
    }
}
