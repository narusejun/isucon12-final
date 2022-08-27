using System;
using System.Text;
using System.Threading.Tasks;
using UnityEngine;
using UnityEngine.Networking;

namespace Network
{
    public partial class ApiClient
    {
        private string _host = "http://localhost:8080";

        public string Host
        {
            get
            {
                return _host;
            }
            set
            {
                if (!string.IsNullOrEmpty(value))
                {
                    _host = value;
                }
            }
        }

        public string ViewerId { get; private set; }
        
        public string SessionId { get; private set; }

        private string _oneTimeToken;
        
        public ApiClient(string host)
        {
            Host = host;
        }

        private async Task<TResponse> Post<TRequest, TResponse>(ApiBase<TRequest, TResponse> api)
            where TRequest : CommonRequest
            where TResponse : CommonResponse
        {
            var url = _host + api.Path;
            Debug.Log("POST url: " + url);
            using var req = new UnityWebRequest(url);
            
            req.method = api.Method;
            req.SetRequestHeader("x-master-version", "1");
            if (SessionId != null)
            {
                req.SetRequestHeader("x-session", SessionId);
            }
            req.downloadHandler = new DownloadHandlerBuffer();
            
            if (api.RequestData != null)
            {
                if (string.IsNullOrEmpty(api.RequestData.viewerId))
                {
                    api.RequestData.viewerId = ViewerId;
                }
                
                req.SetRequestHeader("Content-Type", "application/json");
                var postData = JsonUtility.ToJson(api.RequestData);
                Debug.Log("post data: " + postData);
                req.uploadHandler = new UploadHandlerRaw(Encoding.UTF8.GetBytes(postData));
            }

            await req.SendWebRequest();

            Debug.Log("POST status: " + req.responseCode);
            if (req.result != UnityWebRequest.Result.Success)
            {
                Debug.Log("POST error: " + req.error);
                throw new ApiException((int)req.responseCode, req.error);
            }

            var resText = req.downloadHandler.text;
            Debug.Log("API response: " + resText);
            var response = JsonUtility.FromJson<TResponse>(resText);

            if (response.updatedResources?.user != null && response.updatedResources.user.id != 0)
            {
                GameManager.userData.user.isuCoin = response.updatedResources.user.isuCoin;
                GameManager.userData.isuCoin.refreshTime = response.updatedResources.user.lastGetRewardAt;
            }

            return response;
        }
    }

    public class ApiException : Exception
    {
        public int StatusCode { get; set; }

        public ApiException(int statusCode, string message) : base(message)
        {
            StatusCode = statusCode;
        }

        public override string ToString()
        {
            return $"ApiException: Status={StatusCode}, Message='{Message}'";
        }
    }
}
