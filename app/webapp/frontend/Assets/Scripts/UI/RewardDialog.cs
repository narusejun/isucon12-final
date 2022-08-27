using System;
using Data;
using TMPro;
using UnityEngine;
using UnityEngine.UI;

public class RewardDialog : MonoBehaviour
{
    [SerializeField]
    private TextMeshProUGUI _titleText;

    [SerializeField] private GameObject _rowPrefab;

    [SerializeField] private RectTransform _contentRoot;
    [SerializeField] private Button _closeButton;
    
    public Action onClose;

    private UserPresent[] _presents;

    private void Awake()
    {
        ClearContent();
        _closeButton.onClick.AddListener(() => CloseDialog());
    }

    public void SetData(UserPresent[] presents)
    {
        _presents = presents;
        // ClearContent();
        //
        
        foreach (var present in _presents)
        {
            var go = Instantiate(_rowPrefab, _contentRoot);
            var row = go.GetComponent<RewardRow>();
            row.SetPresent(present);
        }
    }

    private void ClearContent()
    {
        for (int i = 0; i < _contentRoot.childCount; i++)
        {
            Destroy(_contentRoot.GetChild(i).gameObject);
        }
    }

    public void SetTitle(string text)
    {
        _titleText.text = text;
    }

    private void CloseDialog()
    {
        onClose?.Invoke();
    }
}
